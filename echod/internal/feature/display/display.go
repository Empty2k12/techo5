//go:build !dot

// Package display is the Echo Show's screen: what the device shows on it, what a finger on it does,
// and what Home Assistant gets for it.
//
// The screen follows the conversation. Idle, it is a clock; while a turn runs it says what the
// device is doing and shows the words — what was heard, then the answer — and lets them linger a
// while after the turn ends. Everything is drawn by the daemon itself onto the kernel framebuffer
// (hardware/screen): no compositor, no browser, no Android.
//
// A tap does what the Dot's action button does: starts a turn, or ends the one running; on a dark
// screen it only lights it. A vertical swipe is the volume, a notch per step, with the level shown
// while it moves. The room's light dims the panel when auto-brightness is on; what Home Assistant
// sets is the ceiling.
//
// To Home Assistant the screen is a light with brightness only — on/off and how bright, which is
// what people automate — plus a switch for auto-brightness.
package display

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/btaudio"
	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/feature/mute"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/ambient"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/screen"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/touch"
	"github.com/HuskerMinion/techo5/echod/internal/service"
)

func init() {
	// A panel that cannot be opened is retried rather than given up on: the device answers without
	// it, and a boot where the node was late should still end with a screen.
	component.Register(component.Device, Get(), component.Order(60),
		component.Supervise(service.Restart(time.Second, 30*time.Second)))
}

const (
	// linger is how long the last turn's words stay on the screen after it ends.
	linger = 12 * time.Second

	// volumeShow is how long the level stays up after it last moved.
	volumeShow = 2 * time.Second

	// idleFrame and activeFrame are how often the screen is redrawn: once a second for a clock, and
	// fast enough for the listening indicator to breathe and the volume to follow a finger.
	idleFrame   = time.Second
	activeFrame = 150 * time.Millisecond

	// floor is the dimmest an "on" backlight goes; below it the panel reads as off.
	floor = 8

	// Auto-brightness: the fraction of the ceiling the room's light allows, from darkFraction in the
	// dark rising on a log curve to the full ceiling at brightLux. Applied through a running average
	// so a passing shadow does not flicker the panel.
	darkFraction = 0.12
	brightLux    = 400.0
	autoSmooth   = 0.25
)

type Display struct {
	light *esphome.Light
	auto  *esphome.Switch

	mu      sync.Mutex
	on      bool
	ceiling int // percent Home Assistant asked for
	autoOn  bool
	level   float64 // backlight actually applied, 0..BacklightMax, as a running average
	view    voice.State
	viewAt  time.Time
	volume  int
	volAt   time.Time

	poke chan struct{}
	dev  *screen.Device
	r    *renderer

	// booting is the splash: from the first frame until Home Assistant is listening and at least
	// splashMin has passed.
	booting bool
	started time.Time
	logo    *splash

	// sheet is the settings sheet being shown; restartArm is the first of the two taps Restart wants.
	sheet      bool
	restartArm time.Time

	// radio is the radio page being shown.
	radio bool

	// weatherArmed is a weather question in progress; weatherUntil is how long the forecast page
	// stays once the turn is over.
	weatherArmed bool
	weatherUntil time.Time
}

var (
	once   sync.Once
	shared *Display
)

func Get() *Display {
	once.Do(func() { shared = build() })
	return shared
}

func build() *Display {
	d := &Display{
		light: &esphome.Light{
			Base:                esphome.Base{ObjectID: "screen", Name: "Screen", Icon: "mdi:monitor"},
			SupportedColorModes: []esphome.ColorMode{esphome.ColorModeBrightness},
		},
		auto: &esphome.Switch{
			Base: esphome.Base{
				ObjectID: "screen_auto_brightness",
				Name:     "Screen auto-brightness",
				Icon:     "mdi:brightness-auto",
				Category: esphome.CategoryConfig,
			},
		},
		poke: make(chan struct{}, 1),
		view: voice.State{Phase: "idle"},
	}
	d.light.OnCommand = d.command
	d.auto.OnCommand = func(on bool) { d.setAuto(on, true) }
	voice.Changed.Listen(d.changed)
	media.Get().OnVolume.Listen(d.volumeMoved)
	ambient.Get().Lux.Listen(d.lux)
	touch.Get().Gestures.Listen(d.gesture)
	btaudio.Get().Changed.Listen(func(btaudio.State) { d.wake() })
	home.Get().Changed.Listen(func(struct{}) { d.wake() })
	return d
}

func (d *Display) Name() string { return "screen" }

func (d *Display) Entities() []esphome.Entity { return []esphome.Entity{d.light, d.auto} }

// Restore lights the panel the way it was left. Before the framebuffer is opened: the backlight is
// its own device.
func (d *Display) Restore(c config.Config) {
	d.setAuto(c.Screen.Auto, false)
	d.apply(c.Screen.On, c.Screen.Brightness, false)
}

// command is Home Assistant changing the light. A bare "on" carries no brightness; the last one stays.
func (d *Display) command(s esphome.LightState) {
	pct := int(math.Round(float64(s.Brightness) * 100))
	if s.On && s.Brightness == 0 {
		pct = int(math.Round(float64(d.light.Get().Brightness) * 100))
		if pct == 0 {
			pct = config.DefaultScreenBrightness
		}
	}
	d.apply(s.On, pct, true)
}

// apply sets the light's state: the ceiling, and whether the panel is lit at all.
func (d *Display) apply(on bool, pct int, save bool) {
	pct = min(max(pct, 0), 100)
	d.mu.Lock()
	d.on, d.ceiling = on, pct
	d.mu.Unlock()
	d.relight(true)

	d.light.Set(esphome.LightState{On: on, Brightness: float32(pct) / 100, ColorMode: esphome.ColorModeBrightness})
	d.wake()

	if save {
		if err := config.Set().Screen().On(on); err != nil {
			slog.Error("saving the screen state failed", "err", err)
		}
		if err := config.Set().Screen().Brightness(pct); err != nil {
			slog.Error("saving the screen brightness failed", "err", err)
		}
	}
	slog.Info("screen", "on", on, "brightness", pct)
}

func (d *Display) setAuto(on bool, save bool) {
	d.mu.Lock()
	d.autoOn = on
	d.mu.Unlock()
	d.auto.Set(on)
	d.relight(true)
	if save {
		if err := config.Set().Screen().Auto(on); err != nil {
			slog.Error("saving the auto-brightness setting failed", "err", err)
		}
		slog.Info("screen auto-brightness", "on", on)
	}
}

// relight works out the backlight from the ceiling, the room and whether the panel is on, and
// applies it. jump skips the smoothing, for a change the user just asked for.
func (d *Display) relight(jump bool) {
	d.mu.Lock()
	target := 0.0
	if d.on {
		target = float64(d.ceiling) * screen.BacklightMax / 100
		if d.autoOn {
			if lux, _, ok := ambient.Get().Current(); ok {
				target *= allowed(lux)
			}
		}
		target = math.Max(target, floor)
	}
	if jump || d.level == 0 {
		d.level = target
	} else {
		d.level += (target - d.level) * autoSmooth
	}
	level := int(math.Round(d.level))
	d.mu.Unlock()

	if err := screen.SetBacklight(level); err != nil {
		slog.Warn("setting the backlight failed", "err", err)
	}
}

// allowed is the fraction of the ceiling a room this bright gets.
func allowed(lux float64) float64 {
	f := darkFraction + (1-darkFraction)*math.Log10(1+math.Max(lux, 0))/math.Log10(1+brightLux)
	return math.Min(math.Max(f, darkFraction), 1)
}

// lux is a reading from the room. On the sensor's goroutine, twice a second.
func (d *Display) lux(float64) {
	d.mu.Lock()
	auto, on := d.autoOn, d.on
	d.mu.Unlock()
	if auto && on {
		d.relight(false)
	}
}

// changed is the conversation moving on. It runs on the conversation's goroutine, so it only
// records and wakes the loop.
func (d *Display) changed(s voice.State) {
	d.mu.Lock()
	d.view = s
	d.viewAt = time.Now()
	// A question about the weather brings the forecast page up once the answer is done, for a
	// while, and then the screen goes back to whatever it was showing.
	if s.Heard != "" && aboutWeather(s.Heard) {
		d.weatherArmed = true
	}
	if s.Phase == "idle" && d.weatherArmed {
		d.weatherArmed = false
		d.weatherUntil = time.Now().Add(weatherShow)
	}
	d.mu.Unlock()
	d.wake()
}

// aboutWeather is whether what was heard asked about the weather.
func aboutWeather(heard string) bool {
	h := strings.ToLower(heard)
	for _, w := range []string{"weather", "forecast", "temperature", "rain", "snow", "how hot", "how cold"} {
		if strings.Contains(h, w) {
			return true
		}
	}
	return false
}

// volumeMoved is the level changing on purpose; the screen shows it for a moment.
func (d *Display) volumeMoved(step int) {
	d.mu.Lock()
	d.volume, d.volAt = step, time.Now()
	d.mu.Unlock()
	d.wake()
}

// gesture is a finger on the panel. A dark screen only lights up; otherwise a tap is the action
// button and a vertical swipe the volume.
func (d *Display) gesture(g touch.Gesture) {
	d.mu.Lock()
	on := d.on
	d.mu.Unlock()
	slog.Info("touch", "gesture", g.String())

	if !on {
		if g.Kind == touch.Tap {
			d.apply(true, d.ceilingOrDefault(), true)
		}
		return
	}

	// The pairing page: a tap on a row pairs or connects it, the bar at the bottom ends the page.
	// A swipe from the right opens it from the clock.
	bt := btaudio.Get()
	if bt.Pairing() {
		switch g.Kind {
		case touch.Tap:
			if d.r == nil {
				return
			}
			st := bt.State()
			switch row := d.r.btRowAt(g.Y); {
			case row == btRows:
				bt.SetPairing(false)
			case row >= 0 && row < len(st.Devices) && !st.Devices[row].Busy:
				bt.Choose(st.Devices[row].Address)
			}
		case touch.SwipeUp:
			media.Get().Adjust(+1)
		case touch.SwipeDown:
			media.Get().Adjust(-1)
		case touch.SwipeRight:
			bt.SetPairing(false)
		}
		d.wake()
		return
	}

	// The radio page: a row plays (or stops), the bar closes it.
	d.mu.Lock()
	sheet, radio := d.sheet, d.radio
	d.mu.Unlock()
	if radio {
		if g.Kind == touch.Tap && d.r != nil {
			d.radioTap(d.r.radioRowAt(g.Y))
		}
		d.wake()
		return
	}

	// The settings sheet: rows do things, the bar at the bottom closes it.
	if sheet {
		// Vertical swipes do nothing here: the swipe that opened the sheet keeps reporting notches
		// until the finger lifts, and those must not turn into volume steps. The Volume row takes
		// taps on its halves instead.
		if g.Kind == touch.Tap && d.r != nil {
			d.sheetTap(d.r.sheetRowAt(g.Y), g.X)
		}
		d.wake()
		return
	}

	switch g.Kind {
	case touch.Tap:
		d.mu.Lock()
		weatherUp := time.Now().Before(d.weatherUntil)
		idle := d.view.Phase == "idle"
		d.mu.Unlock()
		if weatherUp {
			// The forecast page: a tap puts it away.
			d.mu.Lock()
			d.weatherUntil = time.Time{}
			d.mu.Unlock()
			return
		}
		if idle && d.nowPlaying() {
			// The now-playing screen: a tap is play/pause.
			if playing, _ := media.Get().Playing(); playing {
				media.Get().Pause()
			} else {
				media.Get().Resume()
			}
			return
		}
		voice.Get().Action()
	case touch.SwipeUp:
		media.Get().Adjust(+1)
	case touch.SwipeDown:
		// From the top edge it is the sheet; anywhere else it is the volume.
		if g.Y < topEdge {
			d.showSheet(true)
			return
		}
		media.Get().Adjust(-1)
	}
}

// nowPlaying is whether the idle screen should be the radio's: something playing or paused, and
// the radio wired up so the page has a name to show.
func (d *Display) nowPlaying() bool {
	playing, paused := media.Get().Playing()
	if !playing && !paused {
		return false
	}
	return home.Get().Radio().Configured
}

func (d *Display) showRadio(on bool) {
	d.mu.Lock()
	d.radio = on
	d.mu.Unlock()
	slog.Info("radio page", "open", on)
	d.wake()
}

// radioTap is a finger on a row of the radio page.
func (d *Display) radioTap(row int) {
	if row == radioRows {
		d.showRadio(false)
		return
	}
	if row < 0 {
		return
	}
	rd := home.Get().Radio()
	rows := radioList(rd)
	if row >= len(rows) {
		return
	}
	if rows[row] == "■ Stop" {
		home.Get().Stop()
		return
	}
	home.Get().Play(rows[row])
}

func (d *Display) showSheet(on bool) {
	d.mu.Lock()
	d.sheet = on
	d.restartArm = time.Time{}
	d.mu.Unlock()
	slog.Info("settings sheet", "open", on)
	d.wake()
}

// sheetTap is a finger on a row of the settings sheet; x is where across it landed.
func (d *Display) sheetTap(row, x int) {
	switch row {
	case sheetRows:
		d.showSheet(false)
	case rowRadio:
		d.showSheet(false)
		d.showRadio(true)
	case rowVolume:
		// Left half down, right half up.
		if d.r != nil && x < d.r.w/2 {
			media.Get().Adjust(-1)
		} else {
			media.Get().Adjust(+1)
		}
	case rowBluetooth:
		d.showSheet(false)
		btaudio.Get().SetPairing(true)
	case rowBrightness:
		// Round the dial: 25, 50, 75, 100.
		pct := d.ceilingOrDefault()
		next := (pct/25 + 1) * 25
		if next > 100 {
			next = 25
		}
		d.apply(true, next, true)
	case rowAuto:
		d.mu.Lock()
		on := d.autoOn
		d.mu.Unlock()
		d.setAuto(!on, true)
	case rowMic:
		mute.Get().Toggle()
	case rowRestart:
		d.mu.Lock()
		armed := !d.restartArm.IsZero() && time.Since(d.restartArm) < restartWindow
		if !armed {
			d.restartArm = time.Now()
		}
		d.mu.Unlock()
		if armed {
			slog.Warn("restart asked for from the screen")
			restart()
		}
	}
}

func (d *Display) ceilingOrDefault() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ceiling == 0 {
		return config.DefaultScreenBrightness
	}
	return d.ceiling
}

func (d *Display) wake() {
	select {
	case d.poke <- struct{}{}:
	default:
	}
}

// Start opens the framebuffer.
func (d *Display) Start(context.Context) error {
	dev, err := screen.Open()
	if err != nil {
		return err
	}
	d.dev = dev
	d.r = newRenderer(dev.Canvas())
	w, h := dev.Size()
	d.logo = newSplash(w, h)
	d.mu.Lock()
	d.booting, d.started = true, time.Now()
	d.mu.Unlock()
	slog.Info("screen open", "fb", dev.String())
	return nil
}

func (d *Display) Close() error {
	if d.dev == nil {
		return nil
	}
	err := d.dev.Close()
	d.dev = nil
	return err
}

// Run redraws the screen until ctx is cancelled: on the second while idle, faster while a turn is
// on or the volume is showing, and at once when something changes.
func (d *Display) Run(ctx context.Context) error {
	for {
		wait := d.frame()
		select {
		case <-ctx.Done():
			return nil
		case <-d.poke:
		case <-time.After(wait):
		}
	}
}

// frame draws what the moment calls for and says how long until the next one is due.
func (d *Display) frame() time.Duration {
	d.mu.Lock()
	on, view, at := d.on, d.view, d.viewAt
	volume, volAt := d.volume, d.volAt
	d.mu.Unlock()

	now := time.Now()
	if !on {
		// Dark panel: nothing to draw, and nothing to redraw until told.
		return time.Hour
	}

	d.mu.Lock()
	booting, started := d.booting, d.started
	if booting && now.Sub(started) >= splashMin && voice.Get().Ready() {
		d.booting, booting = false, false
		slog.Info("splash done", "after", now.Sub(started).Round(time.Millisecond))
	}
	d.mu.Unlock()
	if booting {
		d.r.drawSplash(d.logo, now.Sub(started))
		if err := d.dev.Present(); err != nil {
			slog.Warn("presenting the frame failed", "err", err)
		}
		return 80 * time.Millisecond
	}

	s := scene{now: now, phase: view.Phase, heard: view.Heard, reply: view.Reply, since: at}
	if view.Phase == "idle" && (view.Heard != "" || view.Reply != "") && now.Sub(at) < linger {
		s.phase = "lingering"
	}
	s.playing, s.paused = media.Get().Playing()
	s.muted, _ = mute.Get().Muted()
	if !volAt.IsZero() && now.Sub(volAt) < volumeShow {
		s.volume, s.showVolume = volume, true
	}
	s.bt = btaudio.Get().State()
	d.mu.Lock()
	s.showSheet, s.showRadio = d.sheet, d.radio
	restartArm := d.restartArm
	d.mu.Unlock()
	if s.showSheet {
		s.sheet = d.gather(s, restartArm)
	}
	s.nowPlaying = (s.phase == "idle") && d.nowPlaying()
	if s.showRadio || s.nowPlaying {
		s.radio = home.Get().Radio()
	}
	s.weather = home.Get().Weather()
	d.mu.Lock()
	s.showWeather = (s.phase == "idle" || s.phase == "lingering") && now.Before(d.weatherUntil)
	d.mu.Unlock()
	if s.showWeather {
		s.forecast = home.Get().Forecast()
	}

	d.r.draw(s)
	if err := d.dev.Present(); err != nil {
		slog.Warn("presenting the frame failed", "err", err)
	}

	if s.bt.Pairing || s.showSheet || s.showRadio {
		return 500 * time.Millisecond
	}
	if s.showWeather || s.nowPlaying {
		return time.Until(now.Truncate(idleFrame).Add(idleFrame))
	}
	if (s.phase == "idle" || s.phase == "lingering") && !s.showVolume {
		// On the next whole second, so the clock changes when the second does.
		return time.Until(now.Truncate(idleFrame).Add(idleFrame))
	}
	return activeFrame
}
