//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/draw"

	"github.com/HuskerMinion/techo5/echod/internal/feature/btaudio"
)

// The pairing page: a title, a line saying what to do, the devices the scan has found as rows a
// finger can hit, and a bar at the bottom that ends pairing. Geometry is shared with the gesture
// handler, which maps a tap back to a row.
const (
	btRowTop    = 190 // first row's top edge
	btRowHeight = 74
	btRows      = 5
	btDoneBar   = 90 // the bottom bar's height
)

// btRowAt maps a tap to the row it landed on, or -1; the bottom bar is btRows.
func (r *renderer) btRowAt(y int) int {
	if y >= r.h-btDoneBar {
		return btRows
	}
	if y < btRowTop {
		return -1
	}
	row := (y - btRowTop) / btRowHeight
	if row >= btRows {
		return -1
	}
	return row
}

func (r *renderer) pairingPage(s scene) {
	r.cornerClock(s)
	r.text(r.title, "Bluetooth", r.margin, 110, amber)
	hint := s.bt.Status
	if hint == "" {
		hint = "Put your earbuds in pairing mode"
	}
	r.text(r.small, hint, r.margin, 160, dim)

	if len(s.bt.Devices) == 0 {
		// A dot that walks while the scan runs, so a still list reads as searching rather than stuck.
		t := int(s.now.UnixMilli() / 400 % 4)
		r.text(r.body, fmt.Sprintf("Searching%s", "..."[:t]), r.margin, btRowTop+50, dim)
	}
	for i, d := range s.bt.Devices {
		if i >= btRows {
			break
		}
		top := btRowTop + i*btRowHeight
		draw.Draw(r.dst, image.Rect(r.margin, top+btRowHeight-2, r.w-r.margin, top+btRowHeight), image.NewUniform(ember), image.Point{}, draw.Src)
		c := cream
		if d.Busy {
			c = amber
		}
		r.text(r.body, d.Name, r.margin, top+50, c)
		var right string
		switch {
		case d.Busy:
			right = "working…"
		case d.Connected:
			right = "connected"
		case d.Paired:
			right = "paired · tap to connect"
		default:
			right = "tap to pair"
		}
		r.text(r.tiny, right, r.w-r.margin-r.width(r.tiny, right), top+48, dim)
	}

	// The bar that ends it.
	top := r.h - btDoneBar
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(ember), image.Point{}, draw.Src)
	label := "Done"
	r.text(r.title, label, (r.w-r.width(r.title, label))/2, top+62, cream)
}

// btFooter is what the footer says for a connected device.
func btFooter(s btaudio.State) string {
	if s.Connected == "" {
		return ""
	}
	return "BT " + s.Connected
}
