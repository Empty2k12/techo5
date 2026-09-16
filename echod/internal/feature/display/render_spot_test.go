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

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

// The dials: each item's rest puts it at the top, a tap lands on the item under it or the middle, and
// snapping always takes the short way round.
func TestDialGeometry(t *testing.T) {
	for _, items := range [][]menuItem{mainItems, settingsItems} {
		n := len(items)
		for i := range items {
			rot := restFor(i, n)
			if got := topItem(rot, n); got != i {
				t.Errorf("n=%d rest for %d: top is %d", n, i, got)
			}
			x, y := itemPos(i, n, rot)
			if math.Abs(x-centre) > 0.5 || math.Abs(y-(centre-dialR)) > 0.5 {
				t.Errorf("n=%d item %d at rest is at %.1f,%.1f, not the top", n, i, x, y)
			}
			for j := range items {
				jx, jy := itemPos(j, n, rot)
				if got, middle := dialHitAt(int(math.Round(jx)), int(math.Round(jy)), rot, n); middle || got != j {
					t.Errorf("n=%d rot for %d: tap on item %d hit %d (middle %v)", n, i, j, got, middle)
				}
			}
		}
		if d := nearestRest(restFor(0, n), n-1, n) - restFor(0, n); math.Abs(d) > math.Pi {
			t.Errorf("n=%d snapping from 0 to %d turns %.2f rad, the long way", n, n-1, d)
		}
	}
	if _, middle := dialHitAt(centre, centre, 0, len(mainItems)); !middle {
		t.Error("the centre is not the middle")
	}
}

// Every scene draws without panicking; with SPOT_PREVIEW set to a directory, each is written there as
// a PNG to look at.
func TestRoundScenesDraw(t *testing.T) {
	at := time.Date(2026, 9, 16, 14, 7, 0, 0, time.Local)
	sky := home.Weather{Condition: "partlycloudy", Temp: "72°"}
	var week []hass.Day
	for i, c := range []string{"partlycloudy", "rainy", "lightning-rainy", "sunny", "snowy", "cloudy"} {
		week = append(week, hass.Day{When: at.AddDate(0, 0, i), Condition: c, High: float64(78 - 3*i), Low: float64(55 - 2*i), Rain: 10 * i})
	}
	scenes := map[string]roundScene{
		"clock-weather":  {now: at, phase: "idle", weather: sky, timers: []timer.Countdown{{Left: 272 * time.Second, Total: 600 * time.Second, Active: true}}},
		"menu-weather":   {now: at, phase: "idle", weather: sky, menuOpen: true, menuMode: modeMain, menuSel: 4, menuRot: restFor(4, len(mainItems))},
		"weather":        {now: at, phase: "idle", weather: sky, forecast: week, menuOpen: true, menuMode: modeWeather},
		"weather-now":    {now: at, phase: "idle", weather: home.Weather{Condition: "clear-night", Temp: "58°"}, menuOpen: true, menuMode: modeWeather},
		"weather-none":   {now: at, phase: "idle", menuOpen: true, menuMode: modeWeather},
		"clock":          {now: at, phase: "idle", volume: 12, maxVolume: 30},
		"clock-timer":    {now: at, phase: "idle", timers: []timer.Countdown{{Name: "pasta", Left: 4*time.Minute + 32*time.Second, Total: 10 * time.Minute, Active: true}}},
		"muted":          {now: at, phase: "idle", muted: true},
		"listening":      {now: at, phase: "listening"},
		"thinking":       {now: at, phase: "thinking", heard: "what's the weather going to be like this afternoon"},
		"replying":       {now: at, phase: "replying", heard: "what time is it", reply: "It's 2:07 PM. Have a great afternoon, and don't forget the pasta timer is still running in the kitchen."},
		"volume":         {now: at, phase: "idle", volume: 18, maxVolume: 30, showVolume: true},
		"menu":           {now: at, phase: "idle", volume: 12, menuOpen: true, menuMode: modeMain, menuSel: 0, menuRot: restFor(0, len(mainItems))},
		"menu-timers":    {now: at, phase: "idle", menuOpen: true, menuMode: modeMain, menuSel: 5, menuRot: restFor(5, len(mainItems)) + 0.3, timers: []timer.Countdown{{Left: 272 * time.Second, Total: 600 * time.Second, Active: true}}},
		"menu-settings":  {now: at, phase: "idle", menuOpen: true, menuMode: modeSettings, menuSel: 1, menuRot: restFor(1, len(settingsItems)), nightFrom: 22, nightTo: 7, brightness: 72},
		"jog-volume":     {now: at, phase: "idle", menuOpen: true, menuMode: modeVolume, volume: 14, maxVolume: 30},
		"jog-brightness": {now: at, phase: "idle", menuOpen: true, menuMode: modeBrightness, brightness: 72},
		"jog-night":      {now: at, phase: "idle", menuOpen: true, menuMode: modeNightFrom, nightFrom: 22, nightTo: 7},
		"info":           {now: at, phase: "idle", menuOpen: true, menuMode: modeInfo, infoName: "Kitchen", infoAddress: "192.168.1.50", infoVersion: "v0.0.4-spot · slot b"},
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
