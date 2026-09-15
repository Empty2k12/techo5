//go:build !dot

package display

import (
	"image"
	"image/draw"
)

// nowPlaying is the idle screen while the radio plays or sits paused: the weather top left, the
// time top right, the station across the middle, and what a tap does at the bottom. Art and the
// song can join when a source for them exists.
func (r *renderer) nowPlaying(s scene) {
	r.cornerClock(s)
	if w := s.weather; w.Temp != "" || w.Condition != "" {
		line := w.Temp
		if c := conditionWords(w.Condition); c != "" {
			if line != "" {
				line += "  ·  "
			}
			line += c
		}
		r.text(r.small, line, r.margin, r.margin+26, dim)
	}

	rd := s.radio
	station := rd.Now
	if station == "" {
		station = rd.Chosen
	}
	if station == "" {
		station = "Radio"
	}
	label := "Radio"
	if s.paused {
		label = "Paused"
	} else if s.playing {
		label = "Playing"
	}
	r.text(r.small, label, r.margin, 150, amber)

	// The station name, shrunk to fit.
	face := r.title
	if r.width(face, station) > r.w-2*r.margin {
		face = r.body
	}
	lines := r.wrap(face, station, r.w-2*r.margin)
	y := 225
	for i, line := range lines {
		if i == 2 {
			break
		}
		r.text(face, line, r.margin, y, cream)
		y += 56
	}

	// A bar that says what a tap does, with a glyph.
	hint := "tap to pause"
	glyph := "❚❚"
	if s.paused {
		hint, glyph = "tap to play", "▶"
	}
	draw.Draw(r.dst, image.Rect(r.margin, r.h-70, r.w-r.margin, r.h-68), image.NewUniform(ember), image.Point{}, draw.Src)
	r.text(r.body, glyph, r.margin, r.h-30, amber)
	r.text(r.tiny, hint, r.margin+60, r.h-32, dim)
}
