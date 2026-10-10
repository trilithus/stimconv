package analysis

import (
	"bufio"
	"fmt"
	"os"
)

// WriteCSV dumps the per-hop features (100 Hz) for inspection and tuning.
func (f *Features) WriteCSV(path string) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	w := bufio.NewWriter(fh)
	fmt.Fprint(w, "t")
	for _, c := range []string{"A", "B"} {
		for _, k := range []string{"env", "slow", "fast", "carrier_hz", "form_factor", "rhythm_hz", "duty", "attack_s", "jitter", "depth"} {
			fmt.Fprintf(w, ",%s_%s", k, c)
		}
	}
	fmt.Fprintln(w, ",cov_ll,cov_lr,cov_rr,cov_lq")
	n := len(f.CLL)
	for i := 0; i < n; i++ {
		t := float64(i) / HopRate
		fmt.Fprintf(w, "%.2f", t)
		for c := 0; c < 2; c++ {
			ch := &f.Ch[c]
			fmt.Fprintf(w, ",%.5g,%.5g,%.3f,%.1f,%.3f,%.2f,%.3f,%.4f,%.3f,%.3f",
				At(ch.E, EnvRate, t), At(ch.S, EnvRate, t), At(ch.F, EnvRate, t),
				At(ch.Carrier, HopRate, t), At(ch.FormFactor, HopRate, t),
				At(ch.Rate, FrameRate, t), At(ch.Duty, FrameRate, t), At(ch.Attack, FrameRate, t),
				At(ch.Jitter, FrameRate, t), At(ch.Depth, FrameRate, t))
		}
		fmt.Fprintf(w, ",%.5g,%.5g,%.5g,%.5g\n", f.CLL[i], f.CLR[i], f.CRR[i], f.CLQ[i])
	}
	return w.Flush()
}
