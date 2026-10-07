package teleplot

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	got := Parse([]byte("bytes_out:1234\r\nbytes_in:56.5\r\nupdates_sent:60\nx:1:2|g\nlog:hello\nbad\n:3\nxy:1:2:3"))
	want := []Sample{{"bytes_out", 1234}, {"bytes_in", 56.5}, {"updates_sent", 60}, {"x", 2}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestHealth(t *testing.T) {
	now := time.Unix(1000, 0)
	st := NewStore(time.Minute)
	for i := 10; i > 0; i-- {
		st.Add(now.Add(-time.Duration(i)*time.Second), []byte("bytes_in:800\r\nupdates_sent:90"))
	}
	var b bytes.Buffer
	st.Render(&b, now, "", "", 20)
	if !strings.Contains(b.String(), "OK") {
		t.Errorf("steady link not OK:\n%s", b.String())
	}

	st.Add(now, []byte("bytes_in:0\r\nupdates_sent:0"))
	b.Reset()
	st.Render(&b, now, "", "", 20)
	if !strings.Contains(b.String(), "STALL") {
		t.Errorf("stall not reported:\n%s", b.String())
	}

	b.Reset()
	st.Render(&b, now.Add(5*time.Second), "", "", 20)
	if !strings.Contains(b.String(), "disconnected") {
		t.Errorf("silence not reported:\n%s", b.String())
	}
}

func TestRunReceives(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	addr := "127.0.0.1:47391"
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Addr: addr, Window: time.Minute, Refresh: 20 * time.Millisecond, Width: 10}, &out)
	}()
	time.Sleep(50 * time.Millisecond)
	c, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("bytes_in:42"))
	c.Close()
	time.Sleep(100 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "bytes_in") {
		t.Errorf("metric not shown")
	}
}
