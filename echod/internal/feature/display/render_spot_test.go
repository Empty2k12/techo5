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

// The dial: each item's rest puts it at the top, a tap lands on the item under it or the middle, and
// snapping always takes the short way round.
func TestDialGeometry(t *testing.T) {
	for i := range menuItems {
		rot := restFor(i)
		if got := topItem(rot); got != i {
			t.Errorf("rest for %d: top is %d", i, got)
		}
		x, y := itemPos(i, rot)
		if math.Abs(x-centre) > 0.5 || math.Abs(y-(centre-dialR)) > 0.5 {
			t.Errorf("item %d at rest is at %.1f,%.1f, not the top", i, x, y)
		}
		for j := range menuItems {
			jx, jy := itemPos(j, rot)
			if got, middle := dialHitAt(int(math.Round(jx)), int(math.Round(jy)), rot); middle || got != j {
				t.Errorf("rot for %d: tap on item %d hit %d (middle %v)", i, j, got, middle)
			}
		}
	}
	if _, middle := dialHitAt(centre, centre, 0); !middle {
		t.Error("the centre is not the middle")
	}
	if d := nearestRest(restFor(0), 5) - restFor(0); math.Abs(d) > math.Pi {
		t.Errorf("snapping from 0 to 5 turns %.2f rad, the long way", d)
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
		"menu":          {now: at, phase: "idle", volume: 12, menuOpen: true, menuSel: 0, menuRot: restFor(0)},
		"menu-selected": {now: at, phase: "idle", volume: 12, menuOpen: true, menuSel: 2, menuRot: restFor(2), playing: true},
		"menu-turning":  {now: at, phase: "idle", volume: 12, menuOpen: true, menuSel: 1, menuRot: restFor(1) + 0.35},
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
