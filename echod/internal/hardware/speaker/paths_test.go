package speaker

import "testing"

// Steps outside the range clamp rather than panicking on the curve index.
func TestGainForStepClamps(t *testing.T) {
	if got := gainForStep(OutputSpeaker, -5); got != 0 {
		t.Errorf("gainForStep(speaker, -5) = %v, want 0", got)
	}
	if got, top := gainForStep(OutputSpeaker, 99), gainForStep(OutputSpeaker, VolumeSteps); got != top {
		t.Errorf("gainForStep(speaker, 99) = %v, want the top of the curve, %v", got, top)
	}
}

// An unknown output falls back to the speaker curve rather than a zero curve, which would be
// silence.
func TestGainForStepUnknownOutput(t *testing.T) {
	if got, want := gainForStep(Output("bluetooth"), VolumeSteps), gainForStep(OutputSpeaker, VolumeSteps); got != want {
		t.Errorf("gainForStep(bluetooth, top) = %v, want the speaker's %v", got, want)
	}
}
