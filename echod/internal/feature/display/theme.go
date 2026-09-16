//go:build !dot

package display

import (
	"image"
	"image/color"
	"image/draw"
	"log/slog"

	"github.com/HuskerMinion/techo5/echod/internal/config"
)

// Themes: five colours make the whole screen — the ground, the accent, the text, a dim text and
// the rules and boxes. The palette lives in package variables the renderer reads on every frame,
// so switching is a matter of assigning them; the choice is saved with the screen settings.
type theme struct {
	name                             string
	ground, accent, text, dim, rules color.RGBA
}

var themes = []theme{
	{"Walnut", color.RGBA{0x1c, 0x15, 0x11, 0xff}, color.RGBA{0xe9, 0xa2, 0x3b, 0xff}, color.RGBA{0xe8, 0xdc, 0xc8, 0xff}, color.RGBA{0x8a, 0x7d, 0x6c, 0xff}, color.RGBA{0x3a, 0x2c, 0x22, 0xff}},
	{"Slate", color.RGBA{0x14, 0x19, 0x20, 0xff}, color.RGBA{0x5c, 0xb8, 0xff, 0xff}, color.RGBA{0xe4, 0xea, 0xf0, 0xff}, color.RGBA{0x7c, 0x88, 0x96, 0xff}, color.RGBA{0x27, 0x30, 0x3b, 0xff}},
	{"Midnight", color.RGBA{0x08, 0x0a, 0x10, 0xff}, color.RGBA{0x2e, 0xd9, 0xb8, 0xff}, color.RGBA{0xdd, 0xe6, 0xe8, 0xff}, color.RGBA{0x6c, 0x7a, 0x80, 0xff}, color.RGBA{0x18, 0x1e, 0x2a, 0xff}},
	{"Forest", color.RGBA{0x10, 0x1a, 0x14, 0xff}, color.RGBA{0xd8, 0xb4, 0x4a, 0xff}, color.RGBA{0xe6, 0xec, 0xdc, 0xff}, color.RGBA{0x7d, 0x8c, 0x78, 0xff}, color.RGBA{0x22, 0x34, 0x28, 0xff}},
	{"Plum", color.RGBA{0x1a, 0x10, 0x1c, 0xff}, color.RGBA{0xf0, 0x7c, 0xa8, 0xff}, color.RGBA{0xf0, 0xe4, 0xec, 0xff}, color.RGBA{0x8c, 0x74, 0x88, 0xff}, color.RGBA{0x36, 0x24, 0x3c, 0xff}},
	{"Paper", color.RGBA{0xf2, 0xea, 0xdc, 0xff}, color.RGBA{0xb8, 0x5c, 0x1e, 0xff}, color.RGBA{0x2a, 0x22, 0x1c, 0xff}, color.RGBA{0x7a, 0x6e, 0x62, 0xff}, color.RGBA{0xd8, 0xcc, 0xb8, 0xff}},
}

// themeIndex finds a theme by name; unknown names are the first.
func themeIndex(name string) int {
	for i, t := range themes {
		if t.name == name {
			return i
		}
	}
	return 0
}

// applyTheme sets the palette. Called from the display's goroutine only.
func applyTheme(name string) {
	t := themes[themeIndex(name)]
	walnut, amber, cream, dim, ember = t.ground, t.accent, t.text, t.dim, t.rules
}

// nextTheme moves to the next theme and saves it.
func nextTheme() string {
	cur := config.Get().Screen.Theme
	next := themes[(themeIndex(cur)+1)%len(themes)].name
	if err := config.Set().Screen().Theme(next); err != nil {
		slog.Warn("saving the theme failed", "err", err)
	}
	slog.Info("theme", "name", next)
	return next
}

// shift lightens (positive) or darkens (negative) a colour by d per channel.
func shift(c color.RGBA, d int) color.RGBA {
	f := func(v uint8) uint8 {
		n := int(v) + d
		if n < 0 {
			n = 0
		}
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	return color.RGBA{f(c.R), f(c.G), f(c.B), c.A}
}

// dark reports whether the theme is a dark one, which decides which way bevels catch the light.
func dark() bool {
	return int(walnut.R)+int(walnut.G)+int(walnut.B) < 384
}

// bevel draws a box with depth: a fill, a lit top and left edge, a shaded bottom and right edge,
// and a soft shadow under it. raised false sinks the box instead.
func (r *renderer) bevel(rect image.Rectangle, fill color.RGBA, raised bool) {
	light, shadow := shift(fill, 28), shift(fill, -22)
	if !dark() {
		light, shadow = shift(fill, 18), shift(fill, -30)
	}
	if !raised {
		light, shadow = shadow, light
	}
	if raised {
		// The drop shadow, two translucent steps below and to the right.
		sh := image.Rect(rect.Min.X+3, rect.Max.Y, rect.Max.X+3, rect.Max.Y+3)
		draw.Draw(r.dst, sh, image.NewUniform(shade), image.Point{}, draw.Over)
		sh = image.Rect(rect.Max.X, rect.Min.Y+3, rect.Max.X+3, rect.Max.Y)
		draw.Draw(r.dst, sh, image.NewUniform(shade), image.Point{}, draw.Over)
	}
	draw.Draw(r.dst, rect, image.NewUniform(fill), image.Point{}, draw.Src)
	draw.Draw(r.dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+2), image.NewUniform(light), image.Point{}, draw.Src)
	draw.Draw(r.dst, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+2, rect.Max.Y), image.NewUniform(light), image.Point{}, draw.Src)
	draw.Draw(r.dst, image.Rect(rect.Min.X, rect.Max.Y-2, rect.Max.X, rect.Max.Y), image.NewUniform(shadow), image.Point{}, draw.Src)
	draw.Draw(r.dst, image.Rect(rect.Max.X-2, rect.Min.Y, rect.Max.X, rect.Max.Y), image.NewUniform(shadow), image.Point{}, draw.Src)
}
