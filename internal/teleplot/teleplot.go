// Package teleplot is a terminal visualiser for the Teleplot UDP metrics
// restim sends while driving a FOC-Stim (net/teleplot.py: "name:value" lines
// joined by CRLF, one datagram per write_metrics call). It exists to spot a
// degraded WiFi link before it drops: restim skips ticks silently, so a
// falling updates_sent or a stalling bytes_in is the only visible symptom.
package teleplot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAddr is where restim sends its metrics.
const DefaultAddr = "127.0.0.1:47269"

// Sample is one parsed metric value.
type Sample struct {
	Name  string
	Value float64
}

// Parse extracts the numeric samples from one datagram. It accepts restim's
// "name:value" and Teleplot's "name:timestamp:value[|flags]" forms and skips
// text, xy and malformed lines.
func Parse(packet []byte) []Sample {
	var out []Sample
	for _, line := range strings.FieldsFunc(string(packet), func(r rune) bool { return r == '\n' || r == '\r' }) {
		line, _, _ = strings.Cut(line, "|")
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) < 2 || len(parts) > 3 || parts[0] == "" {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(parts[len(parts)-1]), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, Sample{Name: strings.TrimSpace(parts[0]), Value: v})
	}
	return out
}

type point struct {
	t time.Time
	v float64
}

// Series holds the samples of one metric inside the rolling window.
type Series struct {
	points []point
}

func (s *Series) add(t time.Time, v float64) { s.points = append(s.points, point{t, v}) }

func (s *Series) trim(cutoff time.Time) {
	i := sort.Search(len(s.points), func(i int) bool { return !s.points[i].t.Before(cutoff) })
	s.points = append(s.points[:0], s.points[i:]...)
}

// Stats summarises a series over the window.
type Stats struct {
	Last, Min, Max, Mean float64
	Count                int
	Age                  time.Duration // since the last sample
}

func (s *Series) stats(now time.Time) Stats {
	st := Stats{Count: len(s.points), Min: math.Inf(1), Max: math.Inf(-1)}
	if st.Count == 0 {
		return Stats{}
	}
	sum := 0.0
	for _, p := range s.points {
		sum += p.v
		st.Min = math.Min(st.Min, p.v)
		st.Max = math.Max(st.Max, p.v)
	}
	last := s.points[len(s.points)-1]
	st.Last, st.Mean, st.Age = last.v, sum/float64(st.Count), now.Sub(last.t)
	return st
}

// buckets averages the series into n equal time slots ending at now; empty
// slots are NaN.
func (s *Series) buckets(now time.Time, window time.Duration, n int) []float64 {
	sum := make([]float64, n)
	cnt := make([]int, n)
	start := now.Add(-window)
	for _, p := range s.points {
		i := int(float64(p.t.Sub(start)) / float64(window) * float64(n))
		if i >= 0 && i < n {
			sum[i] += p.v
			cnt[i]++
		}
	}
	for i := range sum {
		if cnt[i] == 0 {
			sum[i] = math.NaN()
		} else {
			sum[i] /= float64(cnt[i])
		}
	}
	return sum
}

// Store is the concurrency-safe set of all metrics seen.
type Store struct {
	mu        sync.Mutex
	window    time.Duration
	series    map[string]*Series
	packets   int
	lastPkt   time.Time
	firstSeen time.Time
}

// NewStore keeps samples for the given window.
func NewStore(window time.Duration) *Store {
	return &Store{window: window, series: map[string]*Series{}}
}

// Add records a datagram received at t.
func (st *Store) Add(t time.Time, packet []byte) {
	samples := Parse(packet)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.packets++
	st.lastPkt = t
	if st.firstSeen.IsZero() {
		st.firstSeen = t
	}
	for _, s := range samples {
		ser := st.series[s.Name]
		if ser == nil {
			ser = &Series{}
			st.series[s.Name] = ser
		}
		ser.add(t, s.Value)
	}
}

// Link-health thresholds. restim reports bytes_in/updates_sent once a second,
// and the device streams notifications continuously while playing.
const (
	staleAfter = 2500 * time.Millisecond // a 1 Hz metric missed twice
	commsLost  = 4 * time.Second         // FOC-Stim's own keepalive timeout
)

// health judges the link from restim's 1 Hz byte counters.
func health(now time.Time, lastPkt time.Time, in, sent Stats, haveIn bool) (string, bool) {
	switch {
	case lastPkt.IsZero():
		return "waiting for restim (is FOC-Stim connected and Teleplot enabled?)", true
	case now.Sub(lastPkt) > commsLost:
		return fmt.Sprintf("no metrics for %.0fs: restim stopped or disconnected", now.Sub(lastPkt).Seconds()), false
	case !haveIn:
		return "no bytes_in metric yet", true
	case in.Age > staleAfter:
		return fmt.Sprintf("restim's 1 Hz report is late (%.1fs): its event loop is stalling", in.Age.Seconds()), false
	case in.Last == 0:
		return "STALL: no bytes from the device in the last second", false
	case sent.Count > 0 && sent.Last == 0 && sent.Max > 0:
		return "updates_sent dropped to 0: restim is skipping ticks (link backlog) or playback paused", false
	case in.Max > 0 && in.Min < in.Max*0.25:
		return fmt.Sprintf("spotty: bytes_in dipped to %.0f B/s (peak %.0f) in the window", in.Min, in.Max), false
	}
	return "OK", true
}

// sparkline maps values onto block characters scaled between lo and hi.
func sparkline(vals []float64, lo, hi float64) string {
	const ramp = "▁▂▃▄▅▆▇█"
	glyphs := []rune(ramp)
	var b strings.Builder
	for _, v := range vals {
		switch {
		case math.IsNaN(v):
			b.WriteRune(' ')
		case hi <= lo:
			b.WriteRune(glyphs[0])
		default:
			i := int((v - lo) / (hi - lo) * float64(len(glyphs)-1))
			b.WriteRune(glyphs[max(0, min(len(glyphs)-1, i))])
		}
	}
	return b.String()
}

// priority puts the link metrics first; the rest follow alphabetically.
var priority = map[string]int{"updates_sent": 0, "bytes_in": 1, "bytes_out": 2, "event_loop_latency": 3}

func rank(name, prefix string) int {
	if r, ok := priority[strings.TrimPrefix(name, prefix)]; ok {
		return r
	}
	return len(priority)
}

// Render draws one frame. width is the sparkline width in columns.
func (st *Store) Render(w io.Writer, now time.Time, prefix, filter string, width int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	names := make([]string, 0, len(st.series))
	for name, ser := range st.series {
		ser.trim(now.Add(-st.window))
		if filter == "" || strings.Contains(name, filter) {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := rank(names[i], prefix), rank(names[j], prefix)
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})

	get := func(n string) (Stats, bool) {
		if s := st.series[prefix+n]; s != nil && len(s.points) > 0 {
			return s.stats(now), true
		}
		return Stats{}, false
	}
	in, haveIn := get("bytes_in")
	sent, _ := get("updates_sent")
	msg, ok := health(now, st.lastPkt, in, sent, haveIn)
	color := "\x1b[32m"
	if !ok {
		color = "\x1b[1;31m"
	}

	fmt.Fprintf(w, "stimconv teleplot  |  %d packets  |  window %s  |  Ctrl-C quits\x1b[K\n", st.packets, st.window)
	fmt.Fprintf(w, "link: %s%s\x1b[0m\x1b[K\n\x1b[K\n", color, msg)
	fmt.Fprintf(w, "\x1b[1m%-26s %10s %10s %10s %10s %6s  %s\x1b[0m\x1b[K\n", "metric", "last", "min", "mean", "max", "age", "history (oldest left)")
	for _, name := range names {
		ser := st.series[name]
		s := ser.stats(now)
		age := fmt.Sprintf("%.1fs", s.Age.Seconds())
		if s.Count == 0 {
			age = "-"
		}
		dim := ""
		if s.Count == 0 || s.Age > staleAfter {
			dim = "\x1b[2m"
		}
		line := fmt.Sprintf("%s%-26s %10s %10s %10s %10s %6s  %s\x1b[0m", dim, trunc(name, 26),
			num(s.Last, s.Count), num(s.Min, s.Count), num(s.Mean, s.Count), num(s.Max, s.Count), age,
			sparkline(ser.buckets(now, st.window, width), s.Min, s.Max))
		fmt.Fprintf(w, "%s\x1b[K\n", line)
	}
	if len(names) == 0 && !st.lastPkt.IsZero() {
		fmt.Fprint(w, "(no numeric metrics match)\x1b[K\n")
	}
	fmt.Fprint(w, "\x1b[J")
}

func num(v float64, n int) string {
	if n == 0 {
		return "-"
	}
	a := math.Abs(v)
	switch {
	case a != 0 && (a >= 1e6 || a < 1e-3):
		return strconv.FormatFloat(v, 'g', 4, 64)
	case a >= 100 || v == math.Trunc(v):
		return strconv.FormatFloat(v, 'f', 0, 64)
	default:
		return strconv.FormatFloat(v, 'f', 3, 64)
	}
}

func trunc(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Options configures Run.
type Options struct {
	Addr    string        // UDP listen address
	Window  time.Duration // history kept and shown
	Refresh time.Duration // redraw interval
	Width   int           // sparkline columns
	Prefix  string        // restim's focstim/teleplot_prefix setting
	Filter  string        // show only metrics containing this
	Forward string        // optional address to re-send every datagram to
}

// Run listens until ctx is cancelled, redrawing the terminal.
func Run(ctx context.Context, o Options, out io.Writer) error {
	pc, err := net.ListenPacket("udp", o.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w (is the Teleplot extension or another listener already using it? use --forward to chain them)", o.Addr, err)
	}
	defer pc.Close()

	var fwd net.Conn
	if o.Forward != "" {
		if fwd, err = net.Dial("udp", o.Forward); err != nil {
			return fmt.Errorf("forward to %s: %w", o.Forward, err)
		}
		defer fwd.Close()
	}

	store := NewStore(o.Window)
	go func() {
		<-ctx.Done()
		pc.Close()
	}()
	recvErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 65536)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				recvErr <- err
				return
			}
			store.Add(time.Now(), buf[:n])
			if fwd != nil {
				fwd.Write(buf[:n])
			}
		}
	}()

	enableVT()
	fmt.Fprint(out, "\x1b[?25l\x1b[2J")
	defer fmt.Fprint(out, "\x1b[?25h\n")
	tick := time.NewTicker(o.Refresh)
	defer tick.Stop()
	for {
		var frame strings.Builder
		frame.WriteString("\x1b[H")
		store.Render(&frame, time.Now(), o.Prefix, o.Filter, o.Width)
		io.WriteString(out, frame.String())
		select {
		case <-ctx.Done():
			return nil
		case err := <-recvErr:
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		case <-tick.C:
		}
	}
}
