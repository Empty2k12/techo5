//go:build spot

package display

import (
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
)

// Cameras on the round screen: the Spot's own (Camera on the dial, "show this spot") and Home
// Assistant's ("show the front door", the home_show_camera action). The picture fills the circle,
// cropped from the middle; the Spot's own is mirrored, as a mirror would show it. A tap takes it down,
// a swipe sideways steps to the next camera on the list. While the sensor runs, whatever is on the
// screen, a green dot at the top of the rim says so.

const (
	// cameraShow is how long a camera stays up when asked for; cameraStep when stepped to.
	cameraShow = 30 * time.Second
	cameraStep = 60 * time.Second
)

var colCameraOn = color.RGBA{60, 203, 127, 255}

// stepCamera shows the camera after (or before) the one up, round the list.
func stepCamera(current string, by int) {
	cams := home.Get().Cameras()
	if len(cams) == 0 {
		return
	}
	i := 0
	for k, c := range cams {
		if c.Entity == current {
			i = k
		}
	}
	i = ((i+by)%len(cams) + len(cams)) % len(cams)
	home.Get().ShowCamera(cams[i].Entity, cameraStep)
}

// aboutGoingHome is "go home", "home screen", "main screen": whatever is up comes down.
func aboutGoingHome(heard string) bool {
	h := strings.ToLower(heard)
	return strings.Contains(h, "go home") || strings.Contains(h, "home screen") || strings.Contains(h, "main screen")
}

// cameraView fills the circle with the camera's latest frame.
func (r *roundRenderer) cameraView(s roundScene) {
	r.clear()
	v := s.camera
	if f := v.Frame; f != nil {
		r.coverCircle(f, v.Entity == home.LocalCamera)
	} else {
		msg := "Connecting…"
		if v.Error != "" {
			msg = "No picture"
		}
		r.centred(r.title, msg, 250, colDim)
		if v.Error != "" {
			r.paragraph(r.small, v.Error, 290, colDim, 2)
		}
	}
	// The name on a dark band near the top, readable over any picture.
	if v.Name != "" {
		w := r.width(r.label, v.Name)
		r.line(float64(centre-w/2-10), 62, float64(centre+w/2+10), 62, 32, color.RGBA{0, 0, 0, 150})
		r.centred(r.label, v.Name, 69, colText)
	}
}

// coverCircle draws img scaled to cover the panel, cropped from the middle, inside the rim.
func (r *roundRenderer) coverCircle(img *image.RGBA, mirror bool) {
	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	if sw == 0 || sh == 0 {
		return
	}
	scale := math.Max(float64(side)/float64(sw), float64(side)/float64(sh))
	offX := (float64(sw) - float64(side)/scale) / 2
	offY := (float64(sh) - float64(side)/scale) / 2
	inv := 1 / scale
	rr := float64(rimIn) * float64(rimIn)
	stride := img.Stride
	for y := 0; y < side; y++ {
		dy := float64(y) + 0.5 - centre
		sy := int(offY + (float64(y)+0.5)*inv)
		if sy >= sh {
			sy = sh - 1
		}
		row := img.Pix[sy*stride:]
		for x := 0; x < side; x++ {
			dx := float64(x) + 0.5 - centre
			if dx*dx+dy*dy > rr {
				continue
			}
			sx := int(offX + (float64(x)+0.5)*inv)
			if mirror {
				sx = sw - 1 - sx
			}
			if sx < 0 {
				sx = 0
			} else if sx >= sw {
				sx = sw - 1
			}
			j := (y*side + x) * 4
			k := sx * 4
			r.dst.Pix[j], r.dst.Pix[j+1], r.dst.Pix[j+2], r.dst.Pix[j+3] = row[k], row[k+1], row[k+2], 255
		}
	}
}

// cameraDot is the camera-in-use mark at the top of the rim.
func (r *roundRenderer) cameraDot() {
	r.discAt(centre, float64(centre-(rimIn+rimOut)/2), 9, colIconGround)
	r.discAt(centre, float64(centre-(rimIn+rimOut)/2), 6, colCameraOn)
}
