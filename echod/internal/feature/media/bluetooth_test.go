package media

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/HuskerMinion/techo5/echod/internal/hardware/speaker"
)

func pcm(frames, channels int, sample func(f, ch int) int16) []byte {
	b := make([]byte, frames*channels*2)
	for f := 0; f < frames; f++ {
		for ch := 0; ch < channels; ch++ {
			binary.LittleEndian.PutUint16(b[(f*channels+ch)*2:], uint16(sample(f, ch)))
		}
	}
	return b
}

// 48 kHz stereo is the speaker's own format: passed through untouched.
func TestReceivedAtTheSpeakerRatePassesThrough(t *testing.T) {
	in := pcm(100, 2, func(f, ch int) int16 { return int16(f*10 + ch) })
	out := newToSpeaker(speaker.Rate, 2).run(in)
	if len(out) != 200 {
		t.Fatalf("got %d samples, want 200", len(out))
	}
	for f := 0; f < 100; f++ {
		if out[f*2] != int16(f*10) || out[f*2+1] != int16(f*10+1) {
			t.Fatalf("frame %d changed: %d %d", f, out[f*2], out[f*2+1])
		}
	}
}

// 44.1 kHz, read in uneven pieces, comes out at 48 kHz with the right length and no discontinuity.
func TestReceivedAt44100IsResampledContinuously(t *testing.T) {
	const frames = 44100 // one second
	tone := func(f, ch int) int16 { return int16(8000 * math.Sin(2*math.Pi*440*float64(f)/44100)) }
	in := pcm(frames, 2, tone)

	c := newToSpeaker(44100, 2)
	var out []int16
	for off, piece := 0, 0; off < len(in); piece++ {
		n := (1234 + piece*977) % 5000 * 4
		if n == 0 {
			n = 4
		}
		if off+n > len(in) {
			n = len(in) - off
		}
		out = append(out, c.run(in[off:off+n])...)
		off += n
	}

	got := len(out) / 2
	if got < speaker.Rate-10 || got > speaker.Rate+10 {
		t.Errorf("one second came out as %d frames, want about %d", got, speaker.Rate)
	}
	// A 440 Hz tone at 8000 changes by at most about 2*pi*440/48000*8000 ≈ 461 per output frame.
	for i := 1; i < got; i++ {
		if d := int(out[i*2]) - int(out[(i-1)*2]); d > 520 || d < -520 {
			t.Fatalf("jump of %d at output frame %d: the pieces did not join", d, i)
		}
	}
}

// Mono goes to both channels.
func TestReceivedMonoFillsBothChannels(t *testing.T) {
	in := pcm(480, 1, func(f, _ int) int16 { return int16(f) })
	out := newToSpeaker(speaker.Rate, 1).run(in)
	for i := 0; i+1 < len(out); i += 2 {
		if out[i] != out[i+1] {
			t.Fatalf("channels differ at %d: %d %d", i/2, out[i], out[i+1])
		}
	}
}
