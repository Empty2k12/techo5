//go:build !dot

package display

import (
	"image"
	"image/draw"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
)

// The radio page: what is playing, the stations Home Assistant lists, a Stop row while something
// plays, and the bar that closes the page. Rows fit the 480-row panel: up to seven of 44 from 92.
const (
	radioRowTop    = 92
	radioRowHeight = 44
	radioRows      = 7
	radioDoneBar   = 64
)

// radioRowAt maps a tap to a row, or -1; the bottom bar is radioRows.
func (r *renderer) radioRowAt(y int) int {
	if y >= r.h-radioDoneBar {
		return radioRows
	}
	if y < radioRowTop {
		return -1
	}
	row := (y - radioRowTop) / radioRowHeight
	if row >= radioRows {
		return -1
	}
	return row
}

// radioList is the rows in order: "Stop" first while something plays, then the stations.
func radioList(rd home.Radio) []string {
	var rows []string
	if rd.Playing || rd.Chosen != "" {
		rows = append(rows, "■ Stop")
	}
	return append(rows, rd.Stations...)
}

func (r *renderer) radioPage(s scene) {
	rd := s.radio
	r.text(r.body, "Radio", r.margin, 52, amber)
	t := s.now.Format("3:04")
	r.text(r.small, t, r.w-r.margin-r.width(r.small, t), 52, dim)

	line := "Tap a station"
	switch {
	case !rd.Configured:
		line = "Not set up: call the home_radio action from Home Assistant"
	case rd.Now != "" && rd.Playing:
		line = "Playing  " + rd.Now
	case rd.Chosen != "":
		line = "Starting  " + rd.Chosen + "…"
	case len(rd.Stations) == 0:
		line = "No stations yet"
	}
	r.text(r.tiny, line, r.margin, 84, dim)

	for i, name := range radioList(rd) {
		if i >= radioRows {
			break
		}
		top := radioRowTop + i*radioRowHeight
		draw.Draw(r.dst, image.Rect(r.margin, top+radioRowHeight-1, r.w-r.margin, top+radioRowHeight), image.NewUniform(ember), image.Point{}, draw.Src)
		c := cream
		if name == rd.Now && rd.Playing {
			c = amber
		}
		r.text(r.small, name, r.margin, top+32, c)
	}

	top := r.h - radioDoneBar
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(ember), image.Point{}, draw.Src)
	label := "Done"
	r.text(r.body, label, (r.w-r.width(r.body, label))/2, top+45, cream)
}
