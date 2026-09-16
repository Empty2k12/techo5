//go:build spot

package display

import (
	"fmt"
	"image/color"
	"math"
)

// The ring menu is a dial: the items sit on a circle, the one at the top is chosen, drawn larger in
// its own colour, and named in the middle. A held finger spins the dial; letting go snaps it to the
// nearest item; a tap in the middle (or on the chosen item) does it, a tap on another item turns that
// one to the top.

const (
	dialR      = 165 // radius the items sit on
	dialHub    = 86  // inside this a tap is on the middle
	dialHit    = 42  // how near an item a tap has to land
	chosenRing = 44  // the chosen item's outline
)

type itemID string

const (
	itemTalk       itemID = "talk"
	itemVolumeUp   itemID = "volume_up"
	itemMute       itemID = "mute"
	itemPlayPause  itemID = "play_pause"
	itemVolumeDown itemID = "volume_down"
	itemScreenOff  itemID = "screen_off"
)

type menuItem struct {
	id     itemID
	colour color.RGBA
}

// menuItems go clockwise from the top.
var menuItems = []menuItem{
	{itemTalk, color.RGBA{240, 98, 146, 255}},
	{itemVolumeUp, color.RGBA{58, 160, 255, 255}},
	{itemMute, color.RGBA{229, 72, 77, 255}},
	{itemPlayPause, color.RGBA{60, 203, 127, 255}},
	{itemVolumeDown, color.RGBA{64, 214, 230, 255}},
	{itemScreenOff, color.RGBA{255, 176, 32, 255}},
}

var colIcon = color.RGBA{200, 206, 214, 255}

// itemAngle is where item i sits with the dial at rest, in radians clockwise from straight up.
func itemAngle(i int) float64 { return float64(i) * 2 * math.Pi / float64(len(menuItems)) }

// restFor is the dial rotation that puts item i at the top.
func restFor(i int) float64 { return -itemAngle(i) }

// itemPos is where item i is on the screen with the dial turned by rot.
func itemPos(i int, rot float64) (x, y float64) {
	a := itemAngle(i) + rot
	return centre + dialR*math.Sin(a), centre - dialR*math.Cos(a)
}

// topItem is the item nearest the top with the dial turned by rot.
func topItem(rot float64) int {
	n := len(menuItems)
	step := 2 * math.Pi / float64(n)
	i := int(math.Round(-rot/step)) % n
	if i < 0 {
		i += n
	}
	return i
}

// dialHitAt says what a tap at x, y is on: the middle, an item (its index), or neither (-1, false).
func dialHitAt(x, y int, rot float64) (item int, middle bool) {
	if math.Hypot(float64(x-centre), float64(y-centre)) < dialHub {
		return -1, true
	}
	best, bestD := -1, math.MaxFloat64
	for i := range menuItems {
		ix, iy := itemPos(i, rot)
		if d := math.Hypot(float64(x)-ix, float64(y)-iy); d < dialHit && d < bestD {
			best, bestD = i, d
		}
	}
	return best, false
}

// fingerAngle is the direction of x, y from the centre, clockwise from up.
func fingerAngle(x, y int) float64 { return math.Atan2(float64(x-centre), -float64(y-centre)) }

// wrapAngle brings a in (-π, π].
func wrapAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// nearestRest is the rotation for item i closest to the current one, so snapping never spins the long
// way round.
func nearestRest(rot float64, i int) float64 { return rot + wrapAngle(restFor(i)-rot) }

func itemName(s roundScene, id itemID) string {
	switch id {
	case itemTalk:
		return "Talk"
	case itemVolumeUp:
		return "Louder"
	case itemVolumeDown:
		return "Quieter"
	case itemMute:
		if s.muted {
			return "Unmute"
		}
		return "Mute"
	case itemPlayPause:
		if s.playing {
			return "Pause"
		}
		return "Play"
	case itemScreenOff:
		return "Screen off"
	}
	return ""
}

func itemHint(s roundScene, id itemID) string {
	switch id {
	case itemTalk:
		return "tap to ask"
	case itemVolumeUp, itemVolumeDown:
		return fmt.Sprintf("volume %d", s.volume)
	case itemMute:
		if s.muted {
			return "microphone is off"
		}
		return "microphone is on"
	case itemPlayPause:
		switch {
		case s.playing:
			return "playing"
		case s.paused:
			return "paused"
		}
		return "nothing playing"
	case itemScreenOff:
		return "tap the screen to wake"
	}
	return ""
}

// dial draws the ring menu over a dimmed face.
func (r *roundRenderer) dial(s roundScene) {
	r.dim(236)
	r.ringAt(centre, centre, dialR-1, dialR+1, 0, 2*math.Pi, color.RGBA{62, 68, 78, 255})

	for i, it := range menuItems {
		if i == s.menuSel {
			continue
		}
		x, y := itemPos(i, s.menuRot)
		r.icon(it.id, s, x, y, 18, 2.6, colIcon)
	}
	if s.menuSel >= 0 && s.menuSel < len(menuItems) {
		it := menuItems[s.menuSel]
		x, y := itemPos(s.menuSel, s.menuRot)
		r.discAt(x, y, chosenRing, colBackground)
		r.ringAt(x, y, chosenRing-3.5, chosenRing, 0, 2*math.Pi, it.colour)
		accent := it.colour
		accent.A = 150
		r.ringAt(x, y, chosenRing+6, chosenRing+9, -0.3*math.Pi, 0.55*math.Pi, accent)
		r.icon(it.id, s, x, y, 22, 3.2, it.colour)

		r.centred(r.label, s.now.Format("3:04"), 206, colDim)
		r.centred(r.title, itemName(s, it.id), 252, colText)
		r.centred(r.small, itemHint(s, it.id), 286, colDim)
	}
}

// icon draws one item's line icon, centred at x, y, u half its size, w the stroke width.
func (r *roundRenderer) icon(id itemID, s roundScene, x, y, u, w float64, c color.RGBA) {
	switch id {
	case itemTalk:
		r.micIcon(x, y, u, w, c)
	case itemMute:
		r.micIcon(x, y, u, w, c)
		r.line(x-0.85*u, y-0.85*u, x+0.85*u, y+0.85*u, w, c)
	case itemVolumeUp:
		r.speakerIcon(x, y, u, w, c, true)
	case itemVolumeDown:
		r.speakerIcon(x, y, u, w, c, false)
	case itemPlayPause:
		if s.playing {
			r.line(x-0.32*u, y-0.6*u, x-0.32*u, y+0.6*u, w*1.5, c)
			r.line(x+0.32*u, y-0.6*u, x+0.32*u, y+0.6*u, w*1.5, c)
		} else {
			r.triangle(x-0.4*u, y-0.7*u, x-0.4*u, y+0.7*u, x+0.75*u, y, c)
		}
	case itemScreenOff:
		r.ringAt(x, y, 0.72*u-w/2, 0.72*u+w/2, 0.2*math.Pi, 1.8*math.Pi, c)
		r.line(x, y-0.95*u, x, y-0.2*u, w, c)
	}
}

func (r *roundRenderer) micIcon(x, y, u, w float64, c color.RGBA) {
	hw := 0.34 * u // half the capsule's width
	top, bot := y-0.62*u, y+0.02*u
	r.ringAt(x, top, hw-w/2, hw+w/2, 1.5*math.Pi, 2.5*math.Pi, c) // upper half
	r.ringAt(x, bot, hw-w/2, hw+w/2, 0.5*math.Pi, 1.5*math.Pi, c) // lower half
	r.line(x-hw, top, x-hw, bot, w, c)
	r.line(x+hw, top, x+hw, bot, w, c)
	r.ringAt(x, y-0.1*u, 0.64*u-w/2, 0.64*u+w/2, 0.5*math.Pi, 1.5*math.Pi, c) // the holder
	r.line(x, y+0.54*u, x, y+0.86*u, w, c)
	r.line(x-0.36*u, y+0.86*u, x+0.36*u, y+0.86*u, w, c)
}

func (r *roundRenderer) speakerIcon(x, y, u, w float64, c color.RGBA, plus bool) {
	bx0, bx1, by := x-0.95*u, x-0.5*u, 0.28*u
	r.line(bx0, y-by, bx1, y-by, w, c)
	r.line(bx0, y+by, bx1, y+by, w, c)
	r.line(bx0, y-by, bx0, y+by, w, c)
	r.line(bx1, y-by, x-0.02*u, y-0.72*u, w, c)
	r.line(bx1, y+by, x-0.02*u, y+0.72*u, w, c)
	r.line(x-0.02*u, y-0.72*u, x-0.02*u, y+0.72*u, w, c)
	r.line(x+0.32*u, y, x+0.92*u, y, w, c)
	if plus {
		r.line(x+0.62*u, y-0.3*u, x+0.62*u, y+0.3*u, w, c)
	}
}
