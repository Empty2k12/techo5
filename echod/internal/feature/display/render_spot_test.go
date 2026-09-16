//go:build spot

package display

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
)

// A finger over an item picks it, whichever way round the ring; the hub picks nothing.
func TestMenuAtFollowsTheRing(t *testing.T) {
	if got := menuAt(centre, centre); got != -1 {
		t.Fatalf("centre: got %d, want -1", got)
	}
	for i := range menuItems {
		a := itemAngle(i)
		x := centre + int(math.Round(menuR*math.Sin(a)))
		y := centre - int(math.Round(menuR*math.Cos(a)))
		if got := menuAt(x, y); got != i {
			t.Errorf("item %d at %d,%d: got %d", i, x, y, got)
		}
		// Past the item, towards the rim, is still the item.
		x = centre + int(math.Round(225*math.Sin(a)))
		y = centre - int(math.Round(225*math.Cos(a)))
		if got := menuAt(x, y); got != i {
			t.Errorf("item %d near the rim: got %d", i, got)
		}
	}
}

// Every scene draws without panicking; with SPOT_PREVIEW set to a directory, each is written there as
// a PNG to look at.
func TestRoundScenesDraw(t *testing.T) {
	at := time.Date(2026, 9, 16, 14, 7, 0, 0, time.Local)
	scenes := map[string]roundScene{
		"clock":         {now: at, phase: "idle", volume: 12, maxVolume: 30},
		"clock-timer":   {now: at, phase: "idle", timers: []timer.Countdown{{Name: "pasta", Left: 4*time.Minute + 32*time.Second, Total: 10 * time.Minute, Active: true}}},
		"muted":         {now: at, phase: "idle", muted: true},
		"listening":     {now: at, phase: "listening"},
		"thinking":      {now: at, phase: "thinking", heard: "what's the weather going to be like this afternoon"},
		"replying":      {now: at, phase: "replying", heard: "what time is it", reply: "It's 2:07 PM. Have a great afternoon, and don't forget the pasta timer is still running in the kitchen."},
		"volume":        {now: at, phase: "idle", volume: 18, maxVolume: 30, showVolume: true},
		"menu":          {now: at, phase: "idle", menuOpen: true, menuSel: -1},
		"menu-selected": {now: at, phase: "idle", menuOpen: true, menuSel: 2, playing: true},
	}
	dir := os.Getenv("SPOT_PREVIEW")
	for name, s := range scenes {
		img := image.NewRGBA(image.Rect(0, 0, side, side))
		newRoundRenderer(img).draw(s)
		if dir == "" {
			continue
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
