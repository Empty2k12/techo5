//go:build spot

// Package display is the Echo Spot's round screen: a clock that follows the conversation, the
// volume, timers and the microphone mute around its rim, and a ring menu under a held finger.
//
// Everything is drawn by the daemon onto the kernel framebuffer (hardware/screen), 480×480, as on
// the Show; the layouts are the Spot's own (render_spot.go), because nothing of a 960×480 page fits a
// circle.
//
// Touch: a tap starts or ends a turn (on a dark screen it only lights it); a vertical swipe is the
// volume; a held finger opens the ring menu, and sliding onto an item and letting go picks it. Letting
// go in the middle leaves the menu open for taps, and a tap in the middle closes it.
//
// To Home Assistant the screen is a light with brightness, and a switch for auto-brightness, the
// same entities the Show has.
package display

import (
	"context"
	"image"
	"log/slog"
	"math"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/feature/mute"
	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/ambient"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/buttons"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/screen"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/touch"
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

	// menuIdle closes a ring menu nobody is touching.
	menuIdle = 6 * time.Second

	idleFrame   = time.Second
	activeFrame = 120 * time.Millisecond

	// floor is the dimmest an "on" backlight goes; below it the panel reads as off.
	floor = 8

	// Auto-brightness, as on the Show: from darkFraction of the ceiling in the dark to all of it at
	// brightLux, smoothed.
	darkFraction = 0.12
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

	// menuOpen is the ring menu on the screen; menuSel the item under the finger (-1 none);
	// menuAt the last time anyone touched it; dragging a held finger still down.
	menuOpen bool
	menuSel  int
	menuAt   time.Time
	dragging bool

	poke chan struct{}
	dev  *screen.Device
	r    *roundRenderer
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
		poke:    make(chan struct{}, 1),
		view:    voice.State{Phase: "idle"},
		menuSel: -1,
	}
	d.light.OnCommand = d.command
	d.auto.OnCommand = func(on bool) { d.setAuto(on, true) }
	voice.Changed.Listen(d.changed)
	media.Get().OnVolume.Listen(d.volumeMoved)
	ambient.Get().Lux.Listen(d.lux)
	touch.Get().Gestures.Listen(d.gesture)
	timer.Get().Changed.Listen(func(struct{}) { d.wake() })
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
	d.view = s
	d.viewAt = time.Now()
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

	if open {
		d.menuGesture(g)
		return
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
		d.menuOpen, d.menuSel, d.menuAt, d.dragging = true, -1, time.Now(), true
		d.mu.Unlock()
		d.wake()
	}
}

func (d *Display) menuGesture(g touch.Gesture) {
	item := menuAt(g.X, g.Y)
	d.mu.Lock()
	d.menuAt = time.Now()
	switch g.Kind {
	case touch.Drag:
		d.menuSel = item
		d.mu.Unlock()
		d.wake()
		return
	case touch.Release, touch.Tap:
		d.dragging = false
		if item < 0 {
			if g.Kind == touch.Tap {
				d.menuOpen = false // a tap in the middle closes it
			}
			d.menuSel = -1
			d.mu.Unlock()
			d.wake()
			return
		}
		d.menuOpen, d.menuSel = false, -1
		d.mu.Unlock()
		d.act(item)
		d.wake()
		return
	case touch.SwipeUp:
		d.mu.Unlock()
		media.Get().Adjust(+1)
		return
	case touch.SwipeDown:
		d.mu.Unlock()
		media.Get().Adjust(-1)
		return
	}
	d.mu.Unlock()
}

// act does what a ring menu item says.
func (d *Display) act(item int) {
	switch menuItems[item].id {
	case itemTalk:
		voice.Get().Action()
	case itemVolumeUp:
		media.Get().Adjust(+1)
	case itemVolumeDown:
		media.Get().Adjust(-1)
	case itemMute:
		mute.Get().Toggle()
	case itemPlayPause:
		switch playing, paused := media.Get().Playing(); {
		case playing:
			media.Get().Pause()
		case paused:
			media.Get().Resume()
		}
	case itemScreenOff:
		d.mu.Lock()
		ceiling := d.ceiling
		d.mu.Unlock()
		d.apply(false, ceiling, true)
	}
	slog.Info("ring menu", "item", menuItems[item].id)
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
	if d.menuOpen && !d.dragging && now.Sub(d.menuAt) > menuIdle {
		d.menuOpen, d.menuSel = false, -1
	}
	on, view, at := d.on, d.view, d.viewAt
	s := roundScene{
		now:      now,
		phase:    view.Phase,
		heard:    view.Heard,
		reply:    view.Reply,
		menuOpen: d.menuOpen,
		menuSel:  d.menuSel,
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

	d.r.draw(s)
	if err := d.dev.Present(); err != nil {
		slog.Warn("presenting the frame failed", "err", err)
	}

	switch {
	case s.phase == "listening" || s.phase == "thinking" || s.phase == "replying" || s.showVolume || s.menuOpen:
		return activeFrame
	default:
		return time.Until(now.Truncate(idleFrame).Add(idleFrame))
	}
}

// Screenshot is the canvas as last drawn, for checking a layout from a PC.
func (d *Display) Screenshot() *image.RGBA {
	if d.dev == nil {
		return nil
	}
	src := d.dev.Canvas()
	cp := image.NewRGBA(src.Rect)
	copy(cp.Pix, src.Pix)
	return cp
}
