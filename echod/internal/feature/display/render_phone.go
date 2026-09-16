//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"golang.org/x/image/font"

	"github.com/HuskerMinion/techo5/echod/internal/feature/phone"
)

// The call page: over everything while a call rings, is placed or is up. A title, who it is with, how
// long it has lasted, and the buttons: Decline and Answer while ringing, Hang up otherwise.
const callButtonsTop = ringButtonsTop

// answerGreen and declineRed are the call buttons whatever the theme: every phone uses them, and a
// call is the one page where a wrong tap cannot be taken back.
var (
	answerGreen = color.RGBA{0x2e, 0xa0, 0x4f, 0xff}
	declineRed  = color.RGBA{0xc6, 0x3a, 0x32, 0xff}
)

func (r *renderer) callPage(s scene) {
	st := s.call
	title := "Calling"
	switch {
	case st.Phase == phone.Ringing:
		title = "Incoming call"
	case st.Phase == phone.Talking:
		title = "On a call"
	}
	r.text(r.title, title, (r.w-r.width(r.title, title))/2, 80, amber)

	who := st.Peer
	if who == "" {
		who = "Unknown"
	}
	// As big as fits: a name, or a number with its caller ID name, can be long.
	face := r.body
	for _, f := range []font.Face{r.big, r.title} {
		if r.width(f, who) <= r.w-2*r.margin {
			face = f
			break
		}
	}
	r.text(face, who, (r.w-r.width(face, who))/2, 220, cream)

	if st.Phase == phone.Talking && !st.Since.IsZero() {
		d := s.now.Sub(st.Since).Round(time.Second)
		line := fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
		r.text(r.body, line, (r.w-r.width(r.body, line))/2, 290, dim)
	}

	y0, y1 := callButtonsTop, r.h-30
	if st.Phase == phone.Ringing {
		decline := image.Rect(r.margin, y0, r.w/2-12, y1)
		answer := image.Rect(r.w/2+12, y0, r.w-r.margin, y1)
		r.bevel(decline, declineRed, true)
		r.bevel(answer, answerGreen, true)
		r.text(r.title, "Decline", decline.Min.X+(decline.Dx()-r.width(r.title, "Decline"))/2, y0+76, color.White)
		r.text(r.title, "Answer", answer.Min.X+(answer.Dx()-r.width(r.title, "Answer"))/2, y0+76, color.White)
		return
	}
	hang := image.Rect(r.margin, y0, r.w-r.margin, y1)
	r.bevel(hang, declineRed, true)
	r.text(r.title, "Hang up", hang.Min.X+(hang.Dx()-r.width(r.title, "Hang up"))/2, y0+76, color.White)
}

// callTap is a finger on the call page: the left half of the buttons declines or hangs up, the right
// half answers a call that is ringing.
func (d *Display) callTap(x, y int, st phone.State) {
	if d.r == nil || y < callButtonsTop-20 {
		return
	}
	if st.Phase == phone.Ringing && x >= d.r.w/2 {
		phone.Get().Answer()
		return
	}
	phone.Get().Hangup()
}
