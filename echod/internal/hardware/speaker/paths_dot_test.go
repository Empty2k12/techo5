//go:build dot

package speaker

import (
	"math"
	"testing"
)

// The Dot's vendor curve reaches unity at the top.
func TestGainForStep(t *testing.T) {
	cases := []struct {
		out  Output
		step int
		want float32
	}{
		{OutputSpeaker, 0, 0},
		{OutputSpeaker, 30, 1},
		{OutputSpeaker, 15, 0.398}, // -8 dB
		{OutputSpeaker, 5, 0.0708}, // -23 dB
		{OutputHeadphone, 0, 0},
		{OutputHeadphone, 30, 1},
		{OutputHeadphone, 15, 0.112}, // -19 dB
	}

	for _, c := range cases {
		got := gainForStep(c.out, c.step)
		if math.Abs(float64(got-c.want)) > 0.001 {
			t.Errorf("gainForStep(%s, %d) = %v, want %v", c.out, c.step, got, c.want)
		}
	}
}
