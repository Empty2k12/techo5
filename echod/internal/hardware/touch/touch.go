//go:build !dot

// Package touch owns the Echo Show's touchscreen and says what a finger did: a tap, or a swipe as
// it travels. It does not know what either means; the display decides.
//
// The Goodix controller speaks multitouch protocol B (slots and tracking ids, no single-touch
// axes) in the panel's own portrait frame, 480 wide and 960 tall, while the device sits landscape.
// Coordinates are reported in the landscape frame the screen draws in, using the same quarter turn
// as hardware/screen: landscape x runs along the panel's y, landscape y runs back along the
// panel's x.
package touch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hook"
	"github.com/HuskerMinion/techo5/echod/internal/lib/input"
	"github.com/HuskerMinion/techo5/echod/internal/service"
)

func init() {
	component.Register(component.Hardware, Get(), component.Order(20),
		component.Supervise(service.Restart(time.Second, 30*time.Second)))
}

const (
	deviceName = "goodix-ts"

	absMTSlot       = 0x2f
	absMTPositionX  = 0x35
	absMTPositionY  = 0x36
	absMTTrackingID = 0x39
	synReport       = 0

	// Landscape frame the coordinates are reported in.
	Width  = 960
	Height = 480

	// tapMove is how far a finger may wander and still be a tap; tapHold how long it may stay.
	tapMove = 24
	tapHold = 500 * time.Millisecond

	// notch is how far a vertical swipe travels per step it reports, so a slow drag turns the volume
	// a step at a time and a flick several.
	notch = 40

	// swipeMin is how far a horizontal movement has to go to be a swipe at release.
	swipeMin = 120
)

// Kind is what the finger did.
type Kind string

const (
	Tap        Kind = "tap"
	SwipeUp    Kind = "swipe_up"   // reported per notch while the finger moves
	SwipeDown  Kind = "swipe_down" // likewise
	SwipeLeft  Kind = "swipe_left" // reported once, at release
	SwipeRight Kind = "swipe_right"
)

// Gesture is one thing the finger did, with where it started in the landscape frame.
type Gesture struct {
	Kind Kind
	X, Y int
}

func (g Gesture) String() string { return fmt.Sprintf("%s at %d,%d", g.Kind, g.X, g.Y) }

type Screen struct {
	// Gestures fires as they happen, on the reader's goroutine: listeners must not block.
	Gestures hook.Hook[Gesture]

	dev  *input.Device
	rawW int // panel x range, exclusive
	rawH int // panel y range, exclusive

	mu   sync.Mutex
	down bool // a finger is on the panel
}

var (
	once   sync.Once
	shared *Screen
)

func Get() *Screen {
	once.Do(func() { shared = &Screen{} })
	return shared
}

func (s *Screen) Name() string { return "touchscreen" }

// Touched reports whether a finger is on the panel now.
func (s *Screen) Touched() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.down
}

// Start opens the controller's node and reads its coordinate ranges.
func (s *Screen) Start(context.Context) error {
	dev, err := input.Find(deviceName)
	if err != nil {
		return fmt.Errorf("touch: %w", err)
	}
	x, errX := dev.Abs(absMTPositionX)
	y, errY := dev.Abs(absMTPositionY)
	if errX != nil || errY != nil || x.Max <= x.Min || y.Max <= y.Min {
		// The panel's raw axes, from docs/hardware.md, when the controller will not say.
		x.Min, x.Max, y.Min, y.Max = 0, 479, 0, 959
		slog.Warn("touch: using the documented ranges", "errX", errX, "errY", errY)
	}
	s.dev = dev
	s.rawW = int(x.Max-x.Min) + 1
	s.rawH = int(y.Max-y.Min) + 1
	slog.Info("touchscreen open", "device", dev.Path, "raw", fmt.Sprintf("%dx%d", s.rawW, s.rawH))
	return nil
}

func (s *Screen) Close() error {
	if s.dev == nil {
		return nil
	}
	err := s.dev.Close()
	s.dev = nil
	return err
}

// finger is the one contact being followed: the first slot that went down, until it lifts.
type finger struct {
	slot          int
	id            int32
	x, y          int // raw, current
	sx, sy        int // raw, where it started
	at            time.Time
	notched       int  // steps already reported along the vertical travel
	swiped        bool // a notch went out, so this is not a tap
	seenX, seenY  bool
}

// Run reads until ctx is cancelled; the node is closed from the side to end the blocking read.
func (s *Screen) Run(ctx context.Context) error {
	dev := s.dev
	stop := context.AfterFunc(ctx, func() { _ = dev.Close() })
	defer stop()

	slot := 0
	var f *finger
	// per-slot positions arrive before the slot's tracking id is known to be ours, so keep them all
	type pos struct{ x, y int32 }
	slots := map[int]*pos{}

	for {
		e, err := dev.Read()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("touch: reading %s: %w", dev.Path, err)
		}
		switch e.Type {
		case input.EvAbs:
			p := slots[slot]
			if p == nil {
				p = &pos{}
				slots[slot] = p
			}
			switch e.Code {
			case absMTSlot:
				slot = int(e.Value)
			case absMTPositionX:
				p.x = e.Value
				if f != nil && f.slot == slot {
					f.x, f.seenX = int(e.Value), true
				}
			case absMTPositionY:
				p.y = e.Value
				if f != nil && f.slot == slot {
					f.y, f.seenY = int(e.Value), true
				}
			case absMTTrackingID:
				switch {
				case e.Value >= 0 && f == nil:
					f = &finger{slot: slot, id: e.Value, at: time.Now(), sx: -1}
					s.setDown(true)
				case e.Value < 0 && f != nil && f.slot == slot:
					s.lift(f)
					f = nil
					s.setDown(false)
				}
			}
		case input.EvSyn:
			if e.Code != synReport || f == nil {
				continue
			}
			if f.sx < 0 && f.seenX && f.seenY {
				f.sx, f.sy = f.x, f.y
			}
			if f.sx >= 0 {
				s.moved(f)
			}
		}
	}
}

func (s *Screen) setDown(v bool) {
	s.mu.Lock()
	s.down = v
	s.mu.Unlock()
}

// landscape turns a raw panel point into the frame the screen draws in.
func (s *Screen) landscape(rx, ry int) (x, y int) {
	x = ry * Width / max(s.rawH, 1)
	y = (s.rawW - 1 - rx) * Height / max(s.rawW, 1)
	return min(max(x, 0), Width-1), min(max(y, 0), Height-1)
}

// moved reports vertical travel a notch at a time while the finger is down. "Up" on the landscape
// screen is towards smaller landscape y.
func (s *Screen) moved(f *finger) {
	_, y0 := s.landscape(f.sx, f.sy)
	_, y1 := s.landscape(f.x, f.y)
	steps := (y0 - y1) / notch // positive: finger moved up
	for f.notched < steps {
		f.notched++
		f.swiped = true
		x, y := s.landscape(f.sx, f.sy)
		s.Gestures.Emit(Gesture{Kind: SwipeUp, X: x, Y: y})
	}
	for f.notched > steps {
		f.notched--
		f.swiped = true
		x, y := s.landscape(f.sx, f.sy)
		s.Gestures.Emit(Gesture{Kind: SwipeDown, X: x, Y: y})
	}
}

// lift is the finger leaving: a tap if it barely moved and did not stay, a horizontal swipe if it
// travelled sideways, nothing otherwise (its vertical notches already went out).
func (s *Screen) lift(f *finger) {
	if f.sx < 0 {
		return
	}
	x0, y0 := s.landscape(f.sx, f.sy)
	x1, y1 := s.landscape(f.x, f.y)
	dx, dy := x1-x0, y1-y0
	held := time.Since(f.at)
	switch {
	case !f.swiped && abs(dx) <= tapMove && abs(dy) <= tapMove && held <= tapHold:
		s.Gestures.Emit(Gesture{Kind: Tap, X: x0, Y: y0})
	case !f.swiped && abs(dx) >= swipeMin && abs(dx) > 2*abs(dy):
		k := SwipeRight
		if dx < 0 {
			k = SwipeLeft
		}
		s.Gestures.Emit(Gesture{Kind: k, X: x0, Y: y0})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
