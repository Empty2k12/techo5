//go:build spot

// Package display is the Echo Spot's round screen: a clock that follows the conversation, the
// volume, timers and the microphone mute around its rim, and a ring menu under a held finger.
//
// Everything is drawn by the daemon onto the kernel framebuffer (hardware/screen), 480×480, as on
// the Show; the layouts are the Spot's own (render_spot.go), because nothing of a 960×480 page fits a
// circle.
//
// The weather: the reading under the clock, and a weather face (weather_spot.go) from the dial or after
// a question about the weather.
//
// Touch: a tap starts or ends a turn (on a dark screen it only lights it); a swipe up or down is the
// volume, a step per 60 pixels; a held finger opens the ring menu (menu_spot.go). While the menu is open
// the touch screen follows every moving finger, so dragging round the ring turns the dial (or, for a
// value, is a jog wheel), and a tap in the middle does the item at the top.
//
// The backlight: the panel shows almost nothing below about 120 of 255 and glares at 255, so a
// brightness in percent spans backlightMin to the top. From 22:00 to 07:00 (config Screen.Night) it is
// held to nightCeiling.
//
// To Home Assistant the screen is a light with brightness, and a switch for auto-brightness, the
// same entities the Show has.
package display

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/btaudio"
	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/feature/phone"
	"github.com/HuskerMinion/techo5/echod/internal/feature/mute"
	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/ambient"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/buttons"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/camera"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/screen"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/touch"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
	"github.com/HuskerMinion/techo5/echod/internal/service"
)

func init() {
	component.Register(component.Device, Get(), component.Order(60),
		component.Supervise(service.Restart(time.Second, 30*time.Second)))
}

const (
	// linger is how long the last turn's words stay on the screen after it ends.
	linger = 10 * time.Second

	// volumeShow is how long the level stays up after it last moved.
	volumeShow = 2 * time.Second

	// menuIdle closes a ring menu nobody is touching; jogIdle ends a jog wheel's value the same way;
	// restartWindow is how long the first tap on Restart waits for the second.
	menuIdle      = 6 * time.Second
	jogIdle       = 3 * time.Second
	restartWindow = 4 * time.Second

	// dialFrame is the redraw while the dial turns; dialEase how much of the way to its rest it moves
	// each frame.
	dialFrame = 40 * time.Millisecond
	dialEase  = 0.35

	idleFrame   = time.Second
	activeFrame = 120 * time.Millisecond

	// backlightMin is where this panel starts to be readable: 0 % of brightness lands here. Measured
	// by eye 2026-09-16: 120 looks almost off in a lit room, 191 is fine, 255 is too bright.
	backlightMin = 120

	// nightCeiling is the most brightness the night allows, in percent; defaultNight the hours when
	// nothing is set.
	nightCeiling = 30
	defaultNight = "22-7"

	// Auto-brightness: from darkFraction of the ceiling in the dark to all of it at brightLux,
	// smoothed. Milder than the Show's, because the panel's own range is already narrow.
	darkFraction = 0.6
	brightLux    = 400.0
	autoSmooth   = 0.25
)

type Display struct {
	light *esphome.Light
	auto  *esphome.Switch

	mu      sync.Mutex
	on      bool
	ceiling int
	autoOn  bool
	level   float64
	view    voice.State
	viewAt  time.Time
	volume  int
	volAt   time.Time

	// menuOpen is the ring menu on the screen, menuMode what it shows; menuSel the item at (or turning
	// to) the top; menuRot the dial's rotation now and menuRest where it is heading; menuAt the last
	// touch; spinning a finger turning it, spinAngle its last direction from the centre; jogTurn how
	// far a jog wheel has turned towards its next step; nightFrom and nightTo the hours being set;
	// restartArm the first tap on Restart.
	menuOpen   bool
	menuMode   menuMode
	menuSel    int
	menuRot    float64
	menuRest   float64
	menuAt     time.Time
	spinning   bool
	spinAngle  float64
	jogTurn    float64
	nightFrom  int
	nightTo    int
	restartArm time.Time
	forgetArm  time.Time

	// wasNight is whether the last backlight was set for the night, so the change of hour relights.
	wasNight bool

	// weatherArmed is a weather question in progress; weatherUntil when the weather face comes down.
	weatherArmed bool
	weatherUntil time.Time

	poke chan struct{}

	// shots are screenshot requests, answered with a copy of the next frame once it is drawn whole.
	shots chan chan *image.RGBA
	dev   *screen.Device
	r     *roundRenderer
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
		poke:  make(chan struct{}, 1),
		shots: make(chan chan *image.RGBA, 4),
		view:  voice.State{Phase: "idle"},
	}
	d.light.OnCommand = d.command
	d.auto.OnCommand = func(on bool) { d.setAuto(on, true) }
	voice.Changed.Listen(d.changed)
	media.Get().OnVolume.Listen(d.volumeMoved)
	ambient.Get().Lux.Listen(d.lux)
	touch.Get().Gestures.Listen(d.gesture)
	timer.Get().Changed.Listen(func(struct{}) { d.wake() })
	home.Get().Changed.Listen(func(struct{}) { d.wake() })
	btaudio.Get().Changed.Listen(func(btaudio.State) { d.wake() })
	phone.Get().Changed.Listen(d.callLights)
	// The mute button toggles the mute on the buttons' goroutine; redraw once it has.
	buttons.Get().Events.Listen(func(buttons.Event) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			d.wake()
		}()
	})
	return d
}

func (d *Display) Name() string { return "screen" }

func (d *Display) Entities() []esphome.Entity { return []esphome.Entity{d.light, d.auto} }

// Restore lights the panel the way it was left.
func (d *Display) Restore(c config.Config) {
	d.setAuto(c.Screen.Auto, false)
	d.apply(c.Screen.On, c.Screen.Brightness, false)
}

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
	}
}

func (d *Display) relight(jump bool) {
	night := inNight(time.Now())
	d.mu.Lock()
	d.wasNight = night
	target := 0.0
	if d.on {
		pct := float64(d.ceiling)
		if night {
			pct = math.Min(pct, nightCeiling)
		}
		if d.autoOn {
			if lux, _, ok := ambient.Get().Current(); ok {
				pct *= allowed(lux)
			}
		}
		target = backlightMin + (screen.BacklightMax-backlightMin)*math.Min(math.Max(pct, 0), 100)/100
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

// inNight says whether now is within the night hours, "from-to" in whole hours, wrapping midnight.
func inNight(now time.Time) bool {
	v := config.Get().Screen.Night
	if v == "" {
		v = defaultNight
	}
	var from, to int
	if _, err := fmt.Sscanf(v, "%d-%d", &from, &to); err != nil || from == to {
		return false
	}
	h := now.Hour()
	if from < to {
		return h >= from && h < to
	}
	return h >= from || h < to
}

func allowed(lux float64) float64 {
	f := darkFraction + (1-darkFraction)*math.Log10(1+math.Max(lux, 0))/math.Log10(1+brightLux)
	return math.Min(math.Max(f, darkFraction), 1)
}

func (d *Display) lux(float64) {
	d.mu.Lock()
	auto, on := d.autoOn, d.on
	d.mu.Unlock()
	if auto && on {
		d.relight(false)
	}
}

func (d *Display) changed(s voice.State) {
	d.mu.Lock()
	newHeard := s.Heard != "" && s.Heard != d.view.Heard
	d.view = s
	d.viewAt = time.Now()
	// A question about the weather brings the weather face up once the answer is done.
	if newHeard && aboutWeather(s.Heard) {
		d.weatherArmed = true
	}
	// "Show the front door" goes up at once, while the assistant answers; "go home" takes it down.
	if newHeard {
		if entity := home.Get().MatchCamera(s.Heard); entity != "" {
			d.weatherArmed = false
			go home.Get().ShowCamera(entity, cameraShow)
		}
		if aboutGoingHome(s.Heard) {
			d.weatherArmed = false
			if d.menuOpen {
				d.closeMenu()
			}
			go home.Get().HideCamera()
		}
	}
	if s.Phase == "idle" && d.weatherArmed {
		d.weatherArmed = false
		if !d.menuOpen || d.menuMode == modeWeather {
			d.openMenu(modeWeather, "")
			d.weatherUntil = time.Now().Add(weatherShow)
		}
	}
	d.mu.Unlock()
	d.wake()
}

func (d *Display) volumeMoved(step int) {
	d.mu.Lock()
	d.volume, d.volAt = step, time.Now()
	d.mu.Unlock()
	d.wake()
}

// gesture is a finger on the panel. It runs on the touch reader's goroutine: it records, acts and
// wakes the loop, and never draws.
func (d *Display) gesture(g touch.Gesture) {
	d.mu.Lock()
	on, open := d.on, d.menuOpen
	d.mu.Unlock()

	if !on {
		// A dark panel only lights; nothing under the finger is acted on.
		if g.Kind == touch.Tap || g.Kind == touch.Hold {
			d.apply(true, d.ceilingOrDefault(), true)
		}
		return
	}

	if d.callGesture(g) {
		return
	}
	if open {
		d.menuGesture(g)
		return
	}
	if v, up := home.Get().Camera(); up {
		switch g.Kind {
		case touch.Tap:
			go home.Get().HideCamera()
			return
		case touch.SwipeLeft:
			go stepCamera(v.Entity, +1)
			return
		case touch.SwipeRight:
			go stepCamera(v.Entity, -1)
			return
		}
	}
	switch g.Kind {
	case touch.Tap:
		voice.Get().Action()
	case touch.SwipeUp:
		media.Get().Adjust(+1)
	case touch.SwipeDown:
		media.Get().Adjust(-1)
	case touch.Hold:
		d.mu.Lock()
		d.openMenu(modeMain, itemTalk)
		d.spinning, d.spinAngle = true, fingerAngle(g.X, g.Y)
		d.mu.Unlock()
		d.wake()
	}
}

// openMenu shows a dial with item id at the top, or a value's jog wheel. Called with d.mu held.
func (d *Display) openMenu(mode menuMode, id itemID) {
	d.menuOpen, d.menuMode, d.menuAt, d.jogTurn = true, mode, time.Now(), 0
	if items := itemsFor(mode); items != nil {
		d.menuSel = indexOf(items, id)
		d.menuRot = restFor(d.menuSel, len(items))
		d.menuRest = d.menuRot
	}
	touch.Get().SetFollow(true)
}

// closeMenu takes the menu off the screen. Called with d.mu held.
func (d *Display) closeMenu() {
	d.menuOpen, d.spinning = false, false
	touch.Get().SetFollow(false)
}

func (d *Display) menuGesture(g touch.Gesture) {
	d.mu.Lock()
	d.menuAt = time.Now()
	mode := d.menuMode

	switch {
	case mode.jogging():
		switch g.Kind {
		case touch.Hold:
			d.spinning, d.spinAngle = true, fingerAngle(g.X, g.Y)
		case touch.Drag:
			if !d.spinning {
				break
			}
			a := fingerAngle(g.X, g.Y)
			d.jogTurn += wrapAngle(a - d.spinAngle)
			d.spinAngle = a
			steps := 0
			for d.jogTurn >= jogStep {
				d.jogTurn -= jogStep
				steps++
			}
			for d.jogTurn <= -jogStep {
				d.jogTurn += jogStep
				steps--
			}
			if steps != 0 {
				d.mu.Unlock()
				d.jogBy(mode, steps)
				d.wake()
				return
			}
		case touch.Release:
			d.spinning, d.jogTurn = false, 0
		case touch.Tap:
			d.finishJog(mode)
		}

	case mode == modeInfo:
		if g.Kind == touch.Tap || g.Kind == touch.Release {
			d.openMenu(modeSettings, itemInfo)
		}

	case mode == modeWeather:
		if g.Kind == touch.Tap {
			d.closeMenu()
		} else {
			d.weatherUntil = time.Now().Add(weatherIdle)
		}

	default:
		items := itemsFor(mode)
		n := len(items)
		switch g.Kind {
		case touch.Hold:
			d.spinning, d.spinAngle = true, fingerAngle(g.X, g.Y)
		case touch.Drag:
			if d.spinning {
				a := fingerAngle(g.X, g.Y)
				d.menuRot += wrapAngle(a - d.spinAngle)
				d.spinAngle = a
				d.menuRest = d.menuRot
				d.menuSel = topItem(d.menuRot, n)
			}
		case touch.Release:
			d.spinning = false
			d.menuRest = nearestRest(d.menuRot, d.menuSel, n)
		case touch.Tap:
			item, middle := dialHitAt(g.X, g.Y, d.menuRot, n)
			switch {
			case middle || item == d.menuSel:
				id := items[d.menuSel].id
				d.mu.Unlock()
				d.act(id)
				d.wake()
				return
			case item >= 0:
				d.menuSel = item
				d.menuRest = nearestRest(d.menuRot, item, n)
			}
		case touch.SwipeLeft:
			d.menuSel = (d.menuSel + 1) % n
			d.menuRest = nearestRest(d.menuRot, d.menuSel, n)
		case touch.SwipeRight:
			d.menuSel = (d.menuSel + n - 1) % n
			d.menuRest = nearestRest(d.menuRot, d.menuSel, n)
		}
	}
	d.mu.Unlock()
	d.wake()
}

// jogBy turns a value by steps (clockwise positive).
func (d *Display) jogBy(mode menuMode, steps int) {
	switch mode {
	case modeVolume:
		media.Get().Adjust(steps)
	case modeBrightness:
		pct := min(max(d.ceilingOrDefault()+5*steps, 0), 100)
		d.apply(true, pct, true)
	case modeNightFrom:
		d.mu.Lock()
		d.nightFrom = ((d.nightFrom+steps)%24 + 24) % 24
		d.mu.Unlock()
	case modeNightTo:
		d.mu.Lock()
		d.nightTo = ((d.nightTo+steps)%24 + 24) % 24
		d.mu.Unlock()
	}
}

// finishJog is a tap on a jog wheel: back to the dial it came from, or on to the night's end, or saved.
// Called with d.mu held.
func (d *Display) finishJog(mode menuMode) {
	d.spinning, d.jogTurn = false, 0
	switch mode {
	case modeVolume:
		d.openMenu(modeMain, itemVolume)
	case modeBrightness:
		d.openMenu(modeSettings, itemBrightness)
	case modeNightFrom:
		d.menuMode, d.menuAt = modeNightTo, time.Now()
	case modeNightTo:
		v := fmt.Sprintf("%d-%d", d.nightFrom, d.nightTo)
		d.openMenu(modeSettings, itemNight)
		go func() {
			if err := config.Set().Screen().Night(v); err != nil {
				slog.Error("saving the night hours failed", "err", err)
				return
			}
			slog.Info("night hours", "set", v)
			d.relight(true)
		}()
	}
}

// act does what a dial item says.
func (d *Display) act(id itemID) {
	slog.Info("ring menu", "item", id)
	switch id {
	case itemTalk:
		d.locked(d.closeMenu)
		voice.Get().Action()
	case itemMute:
		mute.Get().Toggle()
	case itemMedia:
		switch playing, paused := media.Get().Playing(); {
		case playing:
			media.Get().Pause()
		case paused:
			media.Get().Resume()
		}
	case itemVolume:
		d.locked(func() { d.openMenu(modeVolume, "") })
	case itemCamera:
		d.locked(d.closeMenu)
		go home.Get().ShowCamera(home.LocalCamera, cameraStep)
	case itemWeather:
		d.locked(func() {
			d.openMenu(modeWeather, "")
			d.weatherUntil = time.Now().Add(weatherIdle)
		})
	case itemTimers:
		if timer.Get().Ringing() {
			timer.Get().Stop()
			d.locked(d.closeMenu)
		}
	case itemSettings:
		d.locked(func() { d.openMenu(modeSettings, itemBrightness) })
	case itemSleep:
		d.locked(d.closeMenu)
		d.mu.Lock()
		ceiling := d.ceiling
		d.mu.Unlock()
		d.apply(false, ceiling, true)
	case itemBrightness:
		d.locked(func() { d.openMenu(modeBrightness, "") })
	case itemNight:
		from, to := nightHours()
		d.locked(func() {
			d.nightFrom, d.nightTo = from, to
			d.openMenu(modeNightFrom, "")
		})
	case itemAuto:
		d.mu.Lock()
		on := d.autoOn
		d.mu.Unlock()
		d.setAuto(!on, true)
	case itemInfo:
		d.locked(func() { d.openMenu(modeInfo, "") })
	case itemRestart:
		d.mu.Lock()
		armed := !d.restartArm.IsZero() && time.Since(d.restartArm) < restartWindow
		if !armed {
			d.restartArm = time.Now()
		}
		d.mu.Unlock()
		if armed {
			slog.Warn("restart asked for from the screen")
			restartDevice()
		}
	case itemBack:
		d.locked(func() {
			if d.menuMode == modeBluetooth {
				d.openMenu(modeSettings, itemBluetooth)
				return
			}
			d.openMenu(modeMain, itemSettings)
		})
	case itemBluetooth:
		d.locked(func() { d.openMenu(modeBluetooth, itemBTPair) })
	case itemBTPair:
		bt := btaudio.Get()
		on := !bt.Pairing()
		go bt.SetPairing(on)
		if on {
			// Pairing picks the strongest speaker it hears on its own; the rim pulses meanwhile.
			d.locked(d.closeMenu)
		}
	case itemBTConnect:
		if btaudio.Get().State().Connected != "" {
			go btaudio.Get().Disconnect()
		} else {
			go btaudio.Get().Reconnect()
		}
	case itemBTForget:
		d.mu.Lock()
		armed := !d.forgetArm.IsZero() && time.Since(d.forgetArm) < restartWindow
		d.forgetArm = time.Time{}
		if !armed {
			d.forgetArm = time.Now()
		}
		d.mu.Unlock()
		if armed {
			go btaudio.Get().Forget()
		}
	}
}

func (d *Display) locked(f func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f()
}

// nightHours is the night as configured, or the default.
func nightHours() (from, to int) {
	v := config.Get().Screen.Night
	if v == "" {
		v = defaultNight
	}
	if _, err := fmt.Sscanf(v, "%d-%d", &from, &to); err != nil {
		fmt.Sscanf(defaultNight, "%d-%d", &from, &to)
	}
	return from, to
}

// restartDevice reboots; the slot store and the daemon's state are on disk already.
func restartDevice() {
	syscall.Sync()
	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		slog.Error("restart failed", "err", err)
	}
}

// deviceAddress is the first IPv4 address that is up and not the loopback.
func deviceAddress() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "no address"
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				return ipn.IP.String()
			}
		}
	}
	return "no address"
}

// bootedSlot is the rootfs slot the initramfs booted, if any.
func bootedSlot() string {
	b, err := os.ReadFile("/run/techo5/slot")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (d *Display) isSpinning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.spinning
}

func (d *Display) ceilingOrDefault() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ceiling > 0 {
		return d.ceiling
	}
	return config.DefaultScreenBrightness
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
	d.r = newRoundRenderer(dev.Canvas())
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

// Run redraws until ctx is cancelled: on the second while idle, faster while something moves.
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

func (d *Display) frame() time.Duration {
	now := time.Now()
	d.mu.Lock()
	nightChanged := d.wasNight != inNight(now)
	d.mu.Unlock()
	if nightChanged {
		d.relight(true)
	}
	d.mu.Lock()
	if d.menuOpen && !d.spinning {
		switch {
		case d.menuMode.jogging() && now.Sub(d.menuAt) > jogIdle:
			d.finishJog(d.menuMode)
		case d.menuMode == modeWeather:
			if now.After(d.weatherUntil) {
				d.closeMenu()
			}
		case !d.menuMode.jogging() && now.Sub(d.menuAt) > menuIdle:
			d.closeMenu()
		}
	}
	turning := false
	if d.menuOpen && !d.spinning {
		if diff := d.menuRest - d.menuRot; math.Abs(diff) > 0.002 {
			d.menuRot += diff * dialEase
			turning = true
		} else {
			d.menuRot = d.menuRest
		}
	}
	on, view, at := d.on, d.view, d.viewAt
	s := roundScene{
		now:          now,
		phase:        view.Phase,
		heard:        view.Heard,
		reply:        view.Reply,
		menuOpen:     d.menuOpen,
		menuMode:     d.menuMode,
		menuSel:      d.menuSel,
		menuRot:      d.menuRot,
		brightness:   d.ceiling,
		autoOn:       d.autoOn,
		nightFrom:    d.nightFrom,
		nightTo:      d.nightTo,
		restartArmed: !d.restartArm.IsZero() && now.Sub(d.restartArm) < restartWindow,
		forgetArmed:  !d.forgetArm.IsZero() && now.Sub(d.forgetArm) < restartWindow,
	}
	if !d.volAt.IsZero() && now.Sub(d.volAt) < volumeShow {
		s.volume, s.showVolume = d.volume, true
	}
	d.mu.Unlock()

	if !on {
		return time.Hour
	}
	if view.Phase == "idle" && (view.Heard != "" || view.Reply != "") && now.Sub(at) < linger {
		s.phase = "lingering"
	}
	s.muted, _ = mute.Get().Muted()
	s.playing, s.paused = media.Get().Playing()
	s.maxVolume = config.VolumeSteps
	if !s.showVolume {
		s.volume = media.Get().Volume()
	}
	for _, t := range timer.Get().List(now) {
		if t.Active {
			s.timers = append(s.timers, t)
		}
	}
	s.timerRinging = timer.Get().Ringing()
	s.call = phone.Get().State()
	s.weather = home.Get().Weather()
	s.camera, s.showCamera = home.Get().Camera()
	s.cameraLive = camera.Get().Running()
	bt := btaudio.Get().State()
	s.btAvailable, s.btPairing, s.btConnected, s.btRemembered, s.btStatus = bt.Available, bt.Pairing, bt.Connected, bt.Remembered, bt.Status
	if s.menuOpen && s.menuMode == modeWeather {
		s.forecast = home.Get().Forecast()
	}
	if s.menuOpen {
		if s.menuMode != modeNightFrom && s.menuMode != modeNightTo {
			s.nightFrom, s.nightTo = nightHours()
		}
		if s.menuMode == modeInfo {
			s.infoName = config.Get().Device.Name
			if s.infoName == "" {
				s.infoName = layout.DefaultName
			}
			s.infoAddress = deviceAddress()
			s.infoVersion = layout.Version
			if slot := bootedSlot(); slot != "" {
				s.infoVersion += " · slot " + slot
			}
		}
	}

	d.r.draw(s)
	if err := d.dev.Present(); err != nil {
		slog.Warn("presenting the frame failed", "err", err)
	}
	for pending := true; pending; {
		select {
		case reply := <-d.shots:
			src := d.dev.Canvas()
			cp := image.NewRGBA(src.Rect)
			copy(cp.Pix, src.Pix)
			reply <- cp
		default:
			pending = false
		}
	}

	switch {
	case turning || (s.menuOpen && d.isSpinning()):
		return dialFrame
	case s.showCamera:
		// New frames wake the loop themselves; this only brings the view down when its time is up.
		return activeFrame
	case s.phase == "listening" || s.phase == "thinking" || s.phase == "replying" || s.showVolume || s.menuOpen || s.btPairing || s.call.Phase != phone.Idle:
		return activeFrame
	default:
		return time.Until(now.Truncate(idleFrame).Add(idleFrame))
	}
}

// Screenshot is the next frame drawn, whole, for checking a layout from a PC; nil if the screen is
// not open or draws nothing within two seconds (a dark panel does not draw).
func (d *Display) Screenshot() *image.RGBA {
	if d.dev == nil {
		return nil
	}
	reply := make(chan *image.RGBA, 1)
	select {
	case d.shots <- reply:
	default:
		return nil
	}
	d.wake()
	select {
	case img := <-reply:
		return img
	case <-time.After(2 * time.Second):
		return nil
	}
}
