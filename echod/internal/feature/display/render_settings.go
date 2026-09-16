//go:build !dot

package display

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"
)

// The settings sheet: a swipe down from the top opens it. Four tabs across the top — Device,
// Bluetooth, Cameras, Radio — rows under them a finger can hit, buttons on the right of the rows
// that do something, and a bar at the bottom that closes the sheet. Geometry is shared with the
// gesture handler. The panel is 960 by 480 in landscape.
const (
	// The tab bar runs from the top to tabBarBottom; sheetRows rows of sheetRowHeight from
	// sheetRowTop end at 408, above the bar at 416.
	tabBarBottom   = 70
	sheetRowTop    = 86
	sheetRowHeight = 40
	sheetRows      = 8
	sheetDoneBar   = 64

	// Buttons sit at the right of a row: a wide one, or a narrow (−) beside it.
	buttonH     = 30
	buttonWide  = 130
	buttonSmall = 70
	buttonGap   = 10

	// topEdge is how far from the top a swipe down has to start to be the sheet rather than the volume.
	// A quarter of the panel: a finger reaching for the top lands 60-100 px down more often than on
	// the bezel (a swipe from y=81 was a volume step on 2026-09-16), and volume swipes start lower.
	topEdge = 120

	// restartWindow is how long a second tap on Restart is honoured after the first.
	restartWindow = 4 * time.Second
)

// Tabs, in order across the top.
const (
	tabDevice = iota
	tabBluetooth
	tabCameras
	tabRadio
	tabs
)

var tabNames = [tabs]string{"Device", "Bluetooth", "Cameras", "Radio"}

// Device tab rows, in order.
const (
	rowVolume = iota
	rowBrightness
	rowAuto
	rowMic
	rowTheme
	rowWake
	rowAbout
	rowRestart
)

// pageOf slices a list for the rows: which items page shows, and whether the last row is the
// "More" row that turns the page. Lists that fit take every row.
func pageOf(n, page int) (start, end int, more bool) {
	if n <= sheetRows {
		return 0, n, false
	}
	per := sheetRows - 1
	pages := (n + per - 1) / per
	page %= pages
	start = page * per
	end = start + per
	if end > n {
		end = n
	}
	return start, end, true
}

// moreRow draws the last row as the page turner.
func (r *renderer) moreRow(n, page int) {
	per := sheetRows - 1
	pages := (n + per - 1) / per
	top := r.row(sheetRows-1, "More", dim)
	r.value(top, fmt.Sprintf("page %d of %d", page%pages+1, pages), 1)
	r.button(top, 2, "Next", false)
}

// Bluetooth tab rows: the device (connected or remembered) and pairing a new one.
const (
	btRowDevice = iota
	btRowPair
)

// hit is where a tap on the sheet landed.
type hit struct {
	tab    int // a tab header, or -1
	row    int // a row, or -1
	button int // 0 none, 1 the narrow left button, 2 the wide right one
	done   bool
}

// settings is what the sheet shows, gathered by the display each frame.
type settings struct {
	tab        int
	page       int // of the open tab's list, when it does not fit
	theme      string
	brightness int // ceiling, percent
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

// sheetHit maps a tap to what it landed on.
func (r *renderer) sheetHit(x, y int) hit {
	h := hit{tab: -1, row: -1}
	switch {
	case y >= r.h-sheetDoneBar:
		h.done = true
	case y < tabBarBottom:
		h.tab = x * tabs / r.w
	case y >= sheetRowTop && (y-sheetRowTop)/sheetRowHeight < sheetRows:
		h.row = (y - sheetRowTop) / sheetRowHeight
		right := r.w - r.margin
		switch {
		case x >= right-buttonWide:
			h.button = 2
		case x >= right-buttonWide-buttonGap-buttonSmall:
			h.button = 1
		}
	}
	return h
}

func (r *renderer) settingsPage(s scene) {
	st := s.sheet
	// Tabs as raised blocks: the open one stands proud in the accent, the rest sit back, sunk into
	// the rules colour. The open tab runs into the content with no rule under it.
	tabW := r.w / tabs
	draw.Draw(r.dst, image.Rect(0, tabBarBottom-2, r.w, tabBarBottom), image.NewUniform(ember), image.Point{}, draw.Src)
	for i, name := range tabNames {
		rect := image.Rect(i*tabW+8, 14, (i+1)*tabW-8, tabBarBottom-2)
		if i == st.tab {
			r.bevel(image.Rect(rect.Min.X, rect.Min.Y-4, rect.Max.X, tabBarBottom), amber, true)
			r.text(r.small, name, i*tabW+(tabW-r.width(r.small, name))/2, 50, walnut)
			continue
		}
		r.bevel(rect, ember, false)
		r.text(r.small, name, i*tabW+(tabW-r.width(r.small, name))/2, 50, dim)
	}

	switch st.tab {
	case tabDevice:
		r.deviceTab(s)
	case tabBluetooth:
		r.bluetoothTab(s)
	case tabCameras:
		r.camerasTab(s)
	case tabRadio:
		r.radioTab(s)
	}

	top := r.h - sheetDoneBar
	draw.Draw(r.dst, image.Rect(0, top, r.w, r.h), image.NewUniform(ember), image.Point{}, draw.Src)
	label := "Done"
	r.text(r.body, label, (r.w-r.width(r.body, label))/2, top+45, cream)
}

// row draws one row's rule and label, and returns its top.
func (r *renderer) row(i int, label string, c color.Color) int {
	top := sheetRowTop + i*sheetRowHeight
	draw.Draw(r.dst, image.Rect(r.margin, top+sheetRowHeight-1, r.w-r.margin, top+sheetRowHeight), image.NewUniform(ember), image.Point{}, draw.Src)
	r.text(r.small, label, r.margin, top+29, c)
	return top
}

// value writes a row's value, right-aligned before its buttons (or the edge when it has none).
func (r *renderer) value(top int, text string, buttons int) {
	right := r.w - r.margin
	switch buttons {
	case 1:
		right -= buttonWide + buttonGap
	case 2:
		right -= buttonWide + buttonGap + buttonSmall + buttonGap
	}
	// Long values are trimmed from the left so the end, which changes, stays visible.
	for r.width(r.tiny, text) > right-r.margin-230 && len(text) > 4 {
		text = "…" + text[4:]
	}
	r.text(r.tiny, text, right-r.width(r.tiny, text), top+28, dim)
}

// button draws a box with a label: slot 1 is the narrow left one, 2 the wide right one.
func (r *renderer) button(top int, slot int, label string, lit bool) {
	right := r.w - r.margin
	x0, x1 := right-buttonWide, right
	if slot == 1 {
		x0, x1 = right-buttonWide-buttonGap-buttonSmall, right-buttonWide-buttonGap
	}
	y0 := top + (sheetRowHeight-buttonH)/2
	fill, ink := shift(ember, 12), color.Color(cream)
	if lit {
		fill, ink = amber, walnut
	}
	r.bevel(image.Rect(x0, y0, x1, y0+buttonH), fill, true)
	r.text(r.tiny, label, x0+(x1-x0-r.width(r.tiny, label))/2, y0+23, ink)
}

func (r *renderer) deviceTab(s scene) {
	st := s.sheet
	top := r.row(rowVolume, "Volume", cream)
	r.value(top, fmt.Sprintf("%d of %d", st.volume, sheetVolumeSteps), 2)
	r.button(top, 1, "−", false)
	r.button(top, 2, "+", false)

	top = r.row(rowBrightness, "Brightness", cream)
	r.value(top, fmt.Sprintf("%d%%", st.brightness), 2)
	r.button(top, 1, "−", false)
	r.button(top, 2, "+", false)

	top = r.row(rowAuto, "Auto-brightness", cream)
	r.value(top, "follows the room's light", 1)
	r.button(top, 2, onOff(st.auto), st.auto)

	top = r.row(rowMic, "Microphone", cream)
	mic, lit := "Listening", false
	if st.muted {
		mic, lit = "Muted", true
	}
	r.value(top, "the mute button does this too", 1)
	r.button(top, 2, mic, lit)

	top = r.row(rowTheme, "Theme", cream)
	r.value(top, st.theme, 1)
	r.button(top, 2, "Next", false)

	top = r.row(rowWake, "Wake word", cream)
	r.value(top, st.wakeWord, 0)

	top = r.row(rowAbout, "About", cream)
	r.value(top, fmt.Sprintf("%s  ·  %s  ·  slot %s  ·  %s", st.name, st.version, st.slot, st.address), 0)

	armed := !st.restartArm.IsZero() && st.now.Sub(st.restartArm) < restartWindow
	top = r.row(rowRestart, "Restart", cream)
	if armed {
		r.value(top, "tap again to restart now", 1)
		r.button(top, 2, "Confirm", true)
	} else {
		r.value(top, "asks twice", 1)
		r.button(top, 2, "Restart", false)
	}
}

func (r *renderer) bluetoothTab(s scene) {
	bt := s.bt
	if !bt.Available {
		r.text(r.tiny, "Bluetooth is not available on this build", r.margin, sheetRowTop+31, dim)
		return
	}
	switch {
	case bt.Connected != "":
		top := r.row(btRowDevice, bt.Connected, amber)
		r.value(top, "connected", 1)
		r.button(top, 2, "Disconnect", false)
	case bt.Remembered != "":
		top := r.row(btRowDevice, bt.Remembered, cream)
		r.value(top, "not connected", 1)
		r.button(top, 2, "Connect", false)
	default:
		top := r.row(btRowDevice, "No device yet", cream)
		r.value(top, "pair earbuds or a speaker below", 0)
	}
	top := r.row(btRowPair, "Pair a new device", cream)
	r.value(top, "put it in pairing mode first", 1)
	r.button(top, 2, "Pair", false)
	if bt.Status != "" {
		r.text(r.tiny, bt.Status, r.margin, sheetRowTop+3*sheetRowHeight+10, dim)
	}
}

func (r *renderer) camerasTab(s scene) {
	if len(s.cameras) == 0 {
		r.text(r.tiny, "Not set up: call the home_cameras action from Home Assistant", r.margin, sheetRowTop+31, dim)
		return
	}
	start, end, more := pageOf(len(s.cameras), s.sheet.page)
	for i, c := range s.cameras[start:end] {
		top := r.row(i, c.Name, cream)
		r.value(top, "or say \"show "+strings.ToLower(c.Name)+"\"", 1)
		r.button(top, 2, "Show", false)
	}
	if more {
		r.moreRow(len(s.cameras), s.sheet.page)
	}
}

func (r *renderer) radioTab(s scene) {
	rd := s.radio
	if !rd.Configured {
		r.text(r.tiny, "Not set up: call the home_radio action from Home Assistant", r.margin, sheetRowTop+31, dim)
		return
	}
	rows := radioList(rd)
	if len(rows) == 0 {
		r.text(r.tiny, "No stations yet", r.margin, sheetRowTop+31, dim)
		return
	}
	start, end, more := pageOf(len(rows), s.sheet.page)
	if more {
		r.moreRow(len(rows), s.sheet.page)
	}
	for i, name := range rows[start:end] {
		c := color.Color(cream)
		label, right := name, "Play"
		if name == "■ Stop" {
			label, right = "Stop", "Stop"
		}
		playing := name == rd.Now && rd.Playing
		switch {
		case playing:
			c, right = amber, "Playing"
		case name == rd.Chosen && rd.Chosen != rd.Now:
			right = "Starting…"
		}
		top := r.row(i, label, c)
		r.button(top, 2, right, playing)
	}
}

func onOff(v bool) string {
	if v {
		return "On"
	}
	return "Off"
}
