//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"math"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/HuskerMinion/techo5/echod/internal/feature/btaudio"
	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
)

// The palette is TECHO5's: walnut ground, amber accent, cream text.
var (
	walnut = color.RGBA{0x1c, 0x15, 0x11, 0xff}
	amber  = color.RGBA{0xe9, 0xa2, 0x3b, 0xff}
	cream  = color.RGBA{0xe8, 0xdc, 0xc8, 0xff}
	dim    = color.RGBA{0x8a, 0x7d, 0x6c, 0xff}
	ember  = color.RGBA{0x3a, 0x2c, 0x22, 0xff}
)

// scene is one frame's worth of facts.
type scene struct {
	now     time.Time
	phase   string // idle, listening, thinking, replying, lingering
	heard   string
	reply   string
	since   time.Time
	playing bool
	paused  bool
	muted   bool

	// volume is shown while it moves: the step out of media.VolumeSteps.
	volume     int
	showVolume bool

	// bt is the Bluetooth audio state: the pairing page replaces everything while it is on, and a
	// connected device is named in the footer.
	bt btaudio.State

	// sheet is the settings sheet, drawn instead of everything else while showSheet is set.
	showSheet bool
	sheet     settings

	// radio is the radio page, likewise; weather is on the clock when known.
	showRadio bool
	radio     home.Radio
	weather   home.Weather
}

const sheetVolumeSteps = media.VolumeSteps

// renderer draws scenes onto one canvas. Faces are made once: parsing a font is cheap, but
// building a face at each size is not something to do per frame.
type renderer struct {
	dst    *image.RGBA
	w, h   int
	clock  font.Face // the big time
	ampm   font.Face
	title  font.Face // "Listening…"
	body   font.Face // transcript and reply
	small  font.Face // date, corner clock, footer
	tiny   font.Face
	margin int
}

func newRenderer(dst *image.RGBA) *renderer {
	r := &renderer{dst: dst, w: dst.Rect.Dx(), h: dst.Rect.Dy(), margin: 40}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		slog.Error("parsing the bold font failed", "err", err)
	}
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		slog.Error("parsing the regular font failed", "err", err)
	}
	face := func(f *opentype.Font, size float64) font.Face {
		if f == nil {
			return nil
		}
		fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			slog.Error("making a font face failed", "size", size, "err", err)
			return nil
		}
		return fc
	}
	r.clock = face(bold, 230)
	r.ampm = face(bold, 56)
	r.title = face(bold, 48)
	r.body = face(regular, 42)
	r.small = face(regular, 34)
	r.tiny = face(regular, 26)
	return r
}

// draw composes a whole frame. Everything is repainted: the canvas is small and a full paint is
// simpler than tracking what changed.
func (r *renderer) draw(s scene) {
	draw.Draw(r.dst, r.dst.Rect, image.NewUniform(walnut), image.Point{}, draw.Src)

	if s.bt.Pairing {
		r.pairingPage(s)
		if s.showVolume {
			r.volumeBar(s)
		}
		return
	}
	if s.showSheet {
		r.settingsPage(s)
		if s.showVolume {
			r.volumeBar(s)
		}
		return
	}
	if s.showRadio {
		r.radioPage(s)
		if s.showVolume {
			r.volumeBar(s)
		}
		return
	}

	switch s.phase {
	case "listening":
		r.status(s, "Listening…", true)
	case "thinking":
		r.status(s, "Thinking…", true)
		r.words(s.heard, "", 200)
	case "replying", "lingering":
		r.cornerClock(s)
		r.words(s.heard, s.reply, 70)
	default:
		r.bigClock(s)
	}
	r.footer(s)
	if s.showVolume {
		r.volumeBar(s)
	}
}

// volumeBar is the level, laid over the bottom of whatever is showing while it moves.
func (r *renderer) volumeBar(s scene) {
	top := r.h - 110
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(walnut), image.Point{}, draw.Src)
	label := "Volume"
	r.text(r.small, label, r.margin, top+38, dim)
	full := r.w - 2*r.margin
	y0 := top + 56
	draw.Draw(r.dst, image.Rect(r.margin, y0, r.margin+full, y0+14), image.NewUniform(ember), image.Point{}, draw.Src)
	fill := full * min(max(s.volume, 0), media.VolumeSteps) / media.VolumeSteps
	draw.Draw(r.dst, image.Rect(r.margin, y0, r.margin+fill, y0+14), image.NewUniform(amber), image.Point{}, draw.Src)
	pct := fmt.Sprintf("%d%%", s.volume*100/media.VolumeSteps)
	r.text(r.small, pct, r.w-r.margin-r.width(r.small, pct), top+38, cream)
}

// bigClock is the idle screen: the time across the middle, the date beneath.
func (r *renderer) bigClock(s scene) {
	hour := s.now.Format("3:04")
	ampm := s.now.Format("PM")
	hw := r.width(r.clock, hour)
	aw := r.width(r.ampm, ampm)
	gap := 18
	x := (r.w - hw - gap - aw) / 2
	base := r.h/2 + 60
	r.text(r.clock, hour, x, base, cream)
	r.text(r.ampm, ampm, x+hw+gap, base, amber)

	date := s.now.Format("Monday, January 2")
	r.text(r.small, date, (r.w-r.width(r.small, date))/2, base+70, dim)

	// The weather, top left, when Home Assistant has told us where to look.
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
}

// conditionWords turns Home Assistant's weather state into words for the screen.
func conditionWords(c string) string {
	switch c {
	case "", "unknown", "unavailable":
		return ""
	case "clear-night":
		return "Clear"
	case "partlycloudy":
		return "Partly cloudy"
	case "lightning-rainy":
		return "Thunderstorms"
	case "snowy-rainy":
		return "Sleet"
	case "exceptional":
		return "Severe"
	}
	// "sunny", "cloudy", "rainy", "pouring", "fog", "hail", "snowy", "windy", "lightning"…
	return strings.ToUpper(c[:1]) + c[1:]
}

// cornerClock keeps the time in view while words have the screen.
func (r *renderer) cornerClock(s scene) {
	t := s.now.Format("3:04 PM")
	r.text(r.small, t, r.w-r.margin-r.width(r.small, t), r.margin+26, dim)
}

// status is a phase title with an indicator that breathes while the device waits.
func (r *renderer) status(s scene, title string, breathe bool) {
	r.cornerClock(s)
	r.text(r.title, title, r.margin, 120, amber)
	if breathe {
		// A bar under the title, its length rising and falling with a period of 1.6 s.
		t := float64(s.now.Sub(s.since).Milliseconds()) / 1600
		f := 0.55 + 0.45*math.Sin(2*math.Pi*t)
		full := r.w - 2*r.margin
		draw.Draw(r.dst, image.Rect(r.margin, 140, r.margin+full, 146), image.NewUniform(ember), image.Point{}, draw.Src)
		draw.Draw(r.dst, image.Rect(r.margin, 140, r.margin+int(float64(full)*f), 146), image.NewUniform(amber), image.Point{}, draw.Src)
	}
}

// words lays out what was heard, dimmed, and the reply beneath it, starting at top and stopping at
// the footer. A long reply is shrunk one step before being cut.
func (r *renderer) words(heard, reply string, top int) {
	y := top
	maxW := r.w - 2*r.margin
	bottom := r.h - 70
	if heard != "" {
		for _, line := range r.wrap(r.small, "“"+heard+"”", maxW) {
			if y+40 > bottom {
				break
			}
			r.text(r.small, line, r.margin, y+30, dim)
			y += 42
		}
		y += 18
	}
	if reply == "" {
		return
	}
	face, lineH := r.body, 52
	lines := r.wrap(face, reply, maxW)
	if len(lines)*lineH > bottom-y {
		face, lineH = r.small, 42
		lines = r.wrap(face, reply, maxW)
	}
	for i, line := range lines {
		if y+lineH > bottom {
			if i > 0 {
				r.text(face, "…", r.margin, y, cream)
			}
			break
		}
		r.text(face, line, r.margin, y+lineH-12, cream)
		y += lineH
	}
}

// footer is the bottom edge: what is playing, and whether the microphones are cut.
func (r *renderer) footer(s scene) {
	y := r.h - 24
	if s.muted {
		r.text(r.tiny, "microphone off", r.margin, y, amber)
	}
	var right string
	switch {
	case s.playing:
		right = "♪ playing"
	case s.paused:
		right = "♪ paused"
	}
	if s.bt.Connected != "" {
		if right != "" {
			right += "  ·  "
		}
		right += "BT " + s.bt.Connected
	}
	if right != "" {
		r.text(r.tiny, right, r.w-r.margin-r.width(r.tiny, right), y, dim)
	}
}

func (r *renderer) text(face font.Face, s string, x, baseline int, c color.Color) {
	if face == nil {
		return
	}
	d := &font.Drawer{Dst: r.dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

func (r *renderer) width(face font.Face, s string) int {
	if face == nil {
		return 0
	}
	return (&font.Drawer{Face: face}).MeasureString(s).Ceil()
}

// wrap breaks text into lines no wider than maxW, on spaces; a single word wider than the line is
// left to overflow rather than split.
func (r *renderer) wrap(face font.Face, s string, maxW int) []string {
	var lines []string
	var line string
	for _, word := range strings.Fields(s) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if line != "" && r.width(face, try) > maxW {
			lines = append(lines, line)
			line = word
			continue
		}
		line = try
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
