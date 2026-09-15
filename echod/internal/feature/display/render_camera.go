//go:build !dot

package display

import (
	"image"
	"image/draw"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
)

// The camera view: the latest frame centred on the panel, the camera's name and the time in the
// corners, and a hint that a tap closes it. Below it, the cameras page: a list, like the radio's.
const (
	camRowTop    = 92
	camRowHeight = 44
	camRows      = 7
	camDoneBar   = 64

	// camListShow is how long a camera picked from the list stays up; cameraVoiceShow one asked
	// for by voice.
	camListShow     = 60 * time.Second
	cameraVoiceShow = 30 * time.Second
)

func (r *renderer) cameraView(s scene, v home.CameraView) {
	if v.Frame != nil {
		b := v.Frame.Bounds()
		x := (r.w - b.Dx()) / 2
		y := (r.h - b.Dy()) / 2
		draw.Draw(r.dst, image.Rect(x, y, x+b.Dx(), y+b.Dy()), v.Frame, b.Min, draw.Src)
	} else {
		msg := "Connecting to " + v.Name + "…"
		if v.Error != "" {
			msg = v.Name + ": " + v.Error
		}
		r.text(r.small, msg, (r.w-r.width(r.small, msg))/2, r.h/2, dim)
	}
	// Corners on a dark strip so they read over any picture.
	draw.Draw(r.dst, image.Rect(0, 0, r.w, 44), image.NewUniform(shade), image.Point{}, draw.Over)
	r.text(r.small, v.Name, r.margin, 32, cream)
	t := s.now.Format("3:04")
	r.text(r.small, t, r.w-r.margin-r.width(r.small, t), 32, dim)
	left := time.Until(v.Until).Round(time.Second)
	hint := "tap to close"
	if left > 0 {
		hint = "tap to close  ·  " + left.String()
	}
	draw.Draw(r.dst, image.Rect(0, r.h-36, r.w, r.h), image.NewUniform(shade), image.Point{}, draw.Over)
	r.text(r.tiny, hint, r.margin, r.h-11, dim)
}

// camRowAt maps a tap on the cameras page to a row, or -1; the bottom bar is camRows.
func (r *renderer) camRowAt(y int) int {
	if y >= r.h-camDoneBar {
		return camRows
	}
	if y < camRowTop {
		return -1
	}
	row := (y - camRowTop) / camRowHeight
	if row >= camRows {
		return -1
	}
	return row
}

func (r *renderer) camerasPage(s scene) {
	r.text(r.body, "Cameras", r.margin, 52, amber)
	t := s.now.Format("3:04")
	r.text(r.small, t, r.w-r.margin-r.width(r.small, t), 52, dim)
	if len(s.cameras) == 0 {
		r.text(r.tiny, "Not set up: call the home_cameras action from Home Assistant", r.margin, 84, dim)
	} else {
		r.text(r.tiny, "Tap a camera, or say \"show the …\"", r.margin, 84, dim)
	}
	for i, c := range s.cameras {
		if i >= camRows {
			break
		}
		top := camRowTop + i*camRowHeight
		draw.Draw(r.dst, image.Rect(r.margin, top+camRowHeight-1, r.w-r.margin, top+camRowHeight), image.NewUniform(ember), image.Point{}, draw.Src)
		r.text(r.small, c.Name, r.margin, top+32, cream)
	}
	top := r.h - camDoneBar
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(ember), image.Point{}, draw.Src)
	label := "Done"
	r.text(r.body, label, (r.w-r.width(r.body, label))/2, top+45, cream)
}
