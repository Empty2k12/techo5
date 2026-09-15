//go:build !dot

package speaker

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/config"
)

// Output is one of the device's audio outputs. The Echo Show 5 has a speaker and no jack, so
// OutputHeadphone exists only so the shared code compiles; DetectOutput never returns it.
type Output string

const (
	OutputSpeaker   Output = "speaker"
	OutputHeadphone Output = "headphone"
)

// The playback ring: the vendor HAL's period at twice its depth.
const (
	period  = 768
	periods = 4
)

// DRAMHold is a second AFE PCM node the player holds open, unconfigured, for as long as it holds
// the playback device.
//
// The MediaTek DL1 driver decides at open() where its ring lives: in the AFE's internal SRAM when
// no other AFE stream is open, in DRAM otherwise. On this Amazon kernel the SRAM path faults on
// the first copy_from_user — a kernel panic and reboot (mtk_pcm_I2S0dl1_copy, fault address
// ffffff8009eb0004), reproduced five times on 2026-09-14 with rings of 12 and 16 KB — while the
// DRAM path plays cleanly. Holding any other AFE node marks the SRAM taken, so DL1 takes DRAM.
// MultiMedia1_Capture is an AFE capture nothing on this device uses.
const DRAMHold = "/dev/snd/pcmC0D1c"

// A mixer write, as the shared player applies it.
type kctl struct {
	name  string
	value string
	level int32
	blob  []byte
}

// On cronos the AIC3101 codec is left as the kernel brings it up, the playback stream is driven
// at unity and the volume curve is applied in software; there is nothing to route, and the
// amplifier switch is never touched (see AmpSwitch below). One write matters: the MAX98396 comes
// up in "Speaker Safe Mode", a power cap that takes about 30 dB off the output (measured
// 2026-09-15: a 0.3 FS tone at the mic went from -47 to -15 dBFS when it was cleared). Amazon's
// HAL cleared it at boot; with Android on the null HAL nobody does, so the daemon does. Codec
// writes are cached until the stream powers up, which is why this goes before the first write.
var initSequence = []kctl{
	{name: "Speaker Safe Mode A", level: 0},
}

var pathSequence = map[Output][]kctl{
	OutputSpeaker:   {},
	OutputHeadphone: {},
}

var headphoneOff = []kctl{}

// jackState is where a headphone switch would be; there is none, so the read fails and the speaker
// is assumed.
const jackState = "/sys/class/switch/h2w/state"

// jackPoll is how often the jack switch is sampled.
const jackPoll = 500 * time.Millisecond

// DetectOutput picks the output to use. A missing switch means no jack detection, so assume the
// speaker.
func DetectOutput() Output {
	b, err := os.ReadFile(jackState)
	if err != nil || strings.TrimSpace(string(b)) == "0" {
		return OutputSpeaker
	}
	return OutputHeadphone
}

// VolumeSteps is the number of volume steps. The range is config's, since that is what a stored
// volume is in.
const VolumeSteps = config.VolumeSteps

// volumeCurves maps a volume step to attenuation in dB: the Dot's vendor speaker curve shape,
// sitting 6 dB below it. With the amplifier out of safe mode this speaker is loud: half the dial
// at the unity-topped curve was "fricking loud" in a small room (2026-09-15), so the whole range
// is pulled down and the top of the dial is room-loud rather than painful.
var volumeCurves = map[Output][VolumeSteps + 1]float64{
	OutputSpeaker: {
		-90, -39, -36, -32, -31, -29, -27, -25, -23, -22,
		-20, -19, -18, -16, -15, -14, -13, -11, -11, -10,
		-10, -10, -10, -9, -9, -9, -9, -9, -8, -7, -6,
	},
	OutputHeadphone: {
		-90, -39, -36, -32, -31, -29, -27, -25, -23, -22,
		-20, -19, -18, -16, -15, -14, -13, -11, -11, -10,
		-10, -10, -10, -9, -9, -9, -9, -9, -8, -7, -6,
	},
}

// mute is the attenuation the curves use for step 0.
const mute = -90

// gainForStep converts a volume step to a linear gain using the output's curve.
func gainForStep(out Output, step int) float32 {
	curve, ok := volumeCurves[out]
	if !ok {
		curve = volumeCurves[OutputSpeaker]
	}
	step = max(0, min(step, VolumeSteps))

	db := curve[step]
	if db <= mute {
		return 0
	}
	return float32(math.Pow(10, db/20))
}

// MediaService is the init service that owns Android's audio HAL on LineageOS.
const MediaService = "vendor.audio-hal"

// AmpSwitch is empty: on cronos Ext_Speaker_Amp_Switch drives the GPIO wired to the MAX98396's
// reset, so switching it off and on resets the amplifier and wipes the register setup the codec
// driver did at probe — which it never repeats, leaving the speaker silent until a reboot
// (found 2026-09-14). The amplifier is left as the kernel brought it up.
const AmpSwitch = ""

// OutputBoost is make-up gain on everything the speaker plays, before the volume curve and the
// limiter. Home Assistant's speech peaks well below full scale and this amplifier, as the kernel
// leaves it, is quiet at unity: the top of the dial was heard as "medium" (2026-09-15). +6 dB.
const OutputBoost = 1.0
