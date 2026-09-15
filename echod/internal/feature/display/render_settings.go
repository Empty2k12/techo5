//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/draw"
	"time"
)

// The settings sheet: a swipe down from the top edge opens it; rows a finger can hit, a bar at
// the bottom closes it. Geometry is shared with the gesture handler. The panel is 960 by 480 in
// landscape, so everything here is laid out for 480 rows.
const (
	sheetRowTop    = 74
	sheetRowHeight = 46
	sheetDoneBar   = 64

	// topEdge is how far from the top a swipe down has to start to be the sheet rather than the volume.
	topEdge = 60

	// restartWindow is how long a second tap on Restart is honoured after the first.
	restartWindow = 4 * time.Second
)

// Sheet rows, in order.
const (
	rowBluetooth = iota
	rowBrightness
	rowAuto
	rowMic
	rowWake
	rowVolume
	rowAbout
	rowRestart
	sheetRows
)

// settings is what the sheet shows, gathered by the display each frame.
type settings struct {
	bluetooth  string // connected device, or what to say instead
	brightness int    // ceiling, percent
	auto       bool
	muted      bool
	wakeWord   string
	volume     int // step out of media.VolumeSteps
	name       string
	version    string
	slot       string
	address    string
	restartArm time.Time // set after a first tap on Restart
	now        time.Time
}

// sheetRowAt maps a tap to the row it landed on, or -1; the bottom bar is sheetRows.
func (r *renderer) sheetRowAt(y int) int {
	if y >= r.h-sheetDoneBar {
		return sheetRows
	}
	if y < sheetRowTop {
		return -1
	}
	row := (y - sheetRowTop) / sheetRowHeight
	if row >= sheetRows {
		return -1
	}
	return row
}

func (r *renderer) settingsPage(s scene) {
	st := s.sheet
	r.text(r.body, "Settings", r.margin, 52, amber)
	t := s.now.Format("3:04")
	r.text(r.small, t, r.w-r.margin-r.width(r.small, t), 52, dim)

	rows := [sheetRows][2]string{}
	rows[rowBluetooth] = [2]string{"Bluetooth", st.bluetooth}
	auto := ""
	if st.auto {
		auto = " · auto"
	}
	rows[rowBrightness] = [2]string{"Brightness", fmt.Sprintf("%d%%%s  ·  tap to change", st.brightness, auto)}
	rows[rowAuto] = [2]string{"Auto-brightness", onOff(st.auto)}
	mic := "listening"
	if st.muted {
		mic = "muted"
	}
	rows[rowMic] = [2]string{"Microphone", mic + "  ·  tap to toggle"}
	rows[rowWake] = [2]string{"Wake word", st.wakeWord}
	rows[rowVolume] = [2]string{"Volume", fmt.Sprintf("%d of %d  ·  swipe up or down", st.volume, sheetVolumeSteps)}
	rows[rowAbout] = [2]string{"About", fmt.Sprintf("%s  ·  %s  ·  slot %s  ·  %s", st.name, st.version, st.slot, st.address)}
	restart := "tap twice"
	if !st.restartArm.IsZero() && st.now.Sub(st.restartArm) < restartWindow {
		restart = "tap again to restart now"
	}
	rows[rowRestart] = [2]string{"Restart", restart}

	for i, row := range rows {
		top := sheetRowTop + i*sheetRowHeight
		draw.Draw(r.dst, image.Rect(r.margin, top+sheetRowHeight-1, r.w-r.margin, top+sheetRowHeight), image.NewUniform(ember), image.Point{}, draw.Src)
		c := cream
		if i == rowRestart && restart != "tap twice" {
			c = amber
		}
		r.text(r.small, row[0], r.margin, top+33, c)
		right := row[1]
		// Long values are trimmed from the left so the end, which changes, stays visible.
		for r.width(r.tiny, right) > r.w-2*r.margin-260 && len(right) > 4 {
			right = "…" + right[4:]
		}
		r.text(r.tiny, right, r.w-r.margin-r.width(r.tiny, right), top+32, dim)
	}

	top := r.h - sheetDoneBar
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(ember), image.Point{}, draw.Src)
	label := "Done"
	r.text(r.body, label, (r.w-r.width(r.body, label))/2, top+45, cream)
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
