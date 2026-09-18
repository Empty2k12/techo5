//go:build !dot && !spot

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

// The checkers path must leave the amplifier enabled (its switch is active low, so Off) and never
// hand the player an AmpSwitch to drive On, and it must connect the RT5616's DAC to the line-out
// and unmute it. cronos clears the MAX98396's safe mode instead and routes nothing.
func TestSpeakerPath(t *testing.T) {
	seq, amp := speakerPath(true)
	if amp != "" {
		t.Errorf("checkers AmpSwitch = %q, want empty: driving it On mutes this board", amp)
	}
	got := map[string]kctl{}
	for _, c := range seq {
		got[c.name] = c
	}
	if c, ok := got["Ext_Speaker_Amp_Switch"]; !ok || c.value != "Off" {
		t.Errorf("Ext_Speaker_Amp_Switch = %+v, want Off", c)
	}
	for _, name := range []string{
		"Stereo DAC MIXL DAC L1 Switch", "Stereo DAC MIXR DAC R1 Switch",
		"OUT MIXL DAC L1 Switch", "OUT MIXR DAC R1 Switch",
		"LOUT MIX OUTVOL L Switch", "LOUT MIX OUTVOL R Switch",
		// Both mutes in LOUT_CTRL1: the output's and the volume stage's. Clearing one is silence.
		"OUT Playback Switch", "OUT Channel Switch",
	} {
		if c, ok := got[name]; !ok || c.level != 1 {
			t.Errorf("%q = %+v, want level 1", name, c)
		}
	}
	for _, name := range []string{"HPO MIX DAC1 Switch", "HP Playback Switch"} {
		if _, ok := got[name]; ok {
			t.Errorf("%q is written; the amplifier is on the line-out, not the headphone pins", name)
		}
	}

	seq, amp = speakerPath(false)
	if amp != "" || len(seq) != 1 || seq[0].name != "Speaker Safe Mode A" {
		t.Errorf("cronos path = %v, %q, want just Speaker Safe Mode A", seq, amp)
	}
}
