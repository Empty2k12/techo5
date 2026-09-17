//go:build spot

package display

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"golang.org/x/image/font"

	"github.com/HuskerMinion/techo5/echod/internal/feature/phone"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/touch"
)

// The call face: over everything while a call rings, is placed or is up. The rim pulses green; a tap
// answers or hangs up (the same as anywhere else on the face, through the action), and a sideways
// swipe declines a call that is ringing.

var colCall = color.RGBA{46, 204, 113, 255}

// callGesture is a finger on the call face, and reports whether it was one.
func (d *Display) callGesture(g touch.Gesture) bool {
	st := phone.Get().State()
	if st.Phase == phone.Idle {
		return false
	}
	switch g.Kind {
	case touch.Tap:
		phone.Get().Button()
	case touch.SwipeLeft, touch.SwipeRight:
		if st.Phase == phone.Ringing {
			phone.Get().Hangup()
		}
	default:
		return false // volume swipes carry on as usual
	}
	d.wake()
	return true
}

// callLights brings a dark panel up for a call that rings: the face is how it is answered.
func (d *Display) callLights(st phone.State) {
	if st.Phase == phone.Ringing {
		d.mu.Lock()
		on := d.on
		d.mu.Unlock()
		if !on {
			d.apply(true, d.ceilingOrDefault(), false)
		}
	}
	d.wake()
}

func (r *roundRenderer) callFace(s roundScene) {
	st := s.call
	pulse := 0.45 + 0.55*math.Abs(math.Sin(float64(s.now.UnixMilli())/400))
	if st.Phase == phone.Talking {
		pulse = 1
	}
	r.arc(rimIn, rimOut, 0, 2*math.Pi, fade(colCall, pulse))

	title := "CALLING"
	switch st.Phase {
	case phone.Ringing:
		title = "INCOMING CALL"
	case phone.Talking:
		title = "ON A CALL"
	}
	r.centred(r.label, title, 150, colCall)

	who := st.Peer
	if who == "" {
		who = "Unknown"
	}
	face := r.small
	for _, f := range []font.Face{r.title, r.body} {
		if r.width(f, who) <= 360 {
			face = f
			break
		}
	}
	r.centred(face, who, 240, colText)

	switch st.Phase {
	case phone.Talking:
		d := s.now.Sub(st.Since).Round(time.Second)
		r.centred(r.body, fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60), 290, colDim)
		r.centred(r.small, "Tap to hang up", 370, colDim)
	case phone.Ringing:
		r.centred(r.small, "Tap to answer", 350, colText)
		r.centred(r.small, "Swipe to decline", 385, colDim)
	default:
		r.centred(r.small, "Tap to hang up", 370, colDim)
	}
}

// contactList is who the Call item offers: a tap calls them.
func (r *roundRenderer) contactList(s roundScene) {
	names := make([]string, len(s.contacts))
	for i, c := range s.contacts {
		names[i] = c.Name
	}
	empty := "No contacts yet: Home Assistant's phone_contacts action sets them"
	if !s.phoneReady {
		empty = "The phone is not set up"
	}
	r.pickList("CALL", names, -1, colCall, empty)
}
