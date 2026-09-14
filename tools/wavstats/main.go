// wavstats reports per-channel levels and how alike channels are, for the multichannel WAVs that
// audioprobe writes. Runs on the host: go run ./tools/wavstats file.wav
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wavstats file.wav")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := stats(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
	}
}

func stats(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return fmt.Errorf("not a RIFF WAVE file")
	}
	channels := int(binary.LittleEndian.Uint16(b[22:]))
	rate := int(binary.LittleEndian.Uint32(b[24:]))
	bits := int(binary.LittleEndian.Uint16(b[34:]))
	if bits != 16 {
		return fmt.Errorf("only 16-bit WAV supported, got %d", bits)
	}
	data := b[44:]
	frames := len(data) / (channels * 2)
	fmt.Printf("%s: %d ch, %d Hz, %d frames (%.2f s)\n", path, channels, rate, frames, float64(frames)/float64(rate))

	ch := make([][]float64, channels)
	for c := range ch {
		ch[c] = make([]float64, frames)
	}
	for f := 0; f < frames; f++ {
		for c := 0; c < channels; c++ {
			v := int16(binary.LittleEndian.Uint16(data[(f*channels+c)*2:]))
			ch[c][f] = float64(v) / 32768
		}
	}
	for c := 0; c < channels; c++ {
		fmt.Printf("  ch%d: rms %6.1f dBFS  peak %6.1f dBFS\n", c, db(rms(ch[c])), db(peak(ch[c])))
	}
	for a := 0; a < channels; a++ {
		for c := a + 1; c < channels; c++ {
			diff := make([]float64, frames)
			for f := range diff {
				diff[f] = ch[a][f] - ch[c][f]
			}
			fmt.Printf("  ch%d vs ch%d: corr %+.4f  diff rms %6.1f dBFS  identical=%v\n", a, c, corr(ch[a], ch[c]), db(rms(diff)), rms(diff) == 0)
		}
	}
	return nil
}

func rms(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}

func peak(x []float64) float64 {
	var p float64
	for _, v := range x {
		if a := math.Abs(v); a > p {
			p = a
		}
	}
	return p
}

func corr(a, b []float64) float64 {
	var sab, saa, sbb float64
	for i := range a {
		sab += a[i] * b[i]
		saa += a[i] * a[i]
		sbb += b[i] * b[i]
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

func db(v float64) float64 {
	if v <= 0 {
		return -120
	}
	return 20 * math.Log10(v)
}
