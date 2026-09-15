//go:build !dot

package mic

// The Echo Show 5 2nd gen (cronos) capture path, TLV320AIC3101 through the MediaTek AFE, opens as
// 16 kHz, S24_3LE, 4 channels. Measured with cmd/audioprobe: ch0 is the microphone, ch1 a
// bit-identical copy of it, ch2 and ch3 the playback loopback, left then right.
const (
	Channels      = 4
	CaptureDevice = 22

	// Mics is how many channels are treated as microphones. The copy in ch1 carries nothing the
	// beamformer could use, so one.
	Mics = 1

	// RefFirst is the first loopback channel and Refs how many follow it, left then right.
	RefFirst = 2
	Refs     = 2

	// CenterMic is the only microphone.
	CenterMic = 0
)

// adcs are the converters the microphone arrives on. The card exposes a single ADC_A.
var adcs = []string{"A"}

// MediaService is the init service that holds the capture device when the daemon is not: on
// LineageOS that is the vendor audio HAL, which opens the microphone path at its own start.
const MediaService = "vendor.audio-hal"
