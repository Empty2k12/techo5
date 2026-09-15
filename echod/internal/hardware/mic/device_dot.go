//go:build dot

package mic

// The Echo Dot 2 (biscuit) capture codec accepts one format only: 16 kHz, S24_3LE, 9 channels.
const (
	Channels      = 9
	CaptureDevice = 24

	// Mics is how many of the nine channels are microphones. ch7 and ch8 are the playback loopback.
	Mics = 7

	// RefFirst is the first loopback channel and Refs how many follow it, left then right.
	RefFirst = 7
	Refs     = 2

	// CenterMic is the middle microphone: no arrival delay relative to the array, and usable with no
	// beamformer at all.
	CenterMic = 6
)

// adcs are the four converters the seven microphones arrive on.
var adcs = []string{"A", "B", "C", "D"}
