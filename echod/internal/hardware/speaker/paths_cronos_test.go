//go:build !dot

package speaker

import (
	"math"
	"testing"
)

// The Show's curve is linear in dB: −45 dB at the first step, −24 dB at half, −6 dB at the top.
func TestGainForStep(t *testing.T) {
	cases := []struct {
		out  Output
		step int
		db   float64
	}{
		{OutputSpeaker, 1, -45},
		{OutputSpeaker, 5, -39},
		{OutputSpeaker, 15, -24},
		{OutputSpeaker, 30, -6},
		{OutputHeadphone, 15, -24},
		{OutputHeadphone, 30, -6},
	}

	for _, c := range cases {
		got := gainForStep(c.out, c.step)
		want := float32(math.Pow(10, c.db/20))
		if math.Abs(float64(got-want)) > 0.001 {
			t.Errorf("gainForStep(%s, %d) = %v, want %v (%v dB)", c.out, c.step, got, want, c.db)
		}
	}
	for _, out := range []Output{OutputSpeaker, OutputHeadphone} {
		if got := gainForStep(out, 0); got != 0 {
			t.Errorf("gainForStep(%s, 0) = %v, want silence", out, got)
		}
	}
}
