//go:build !dot

package camera

import (
	"context"
	"image/png"
	"net/http"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/display"
	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
)

// registerScreen adds /screen.png: what the panel shows. ?sheet= opens the settings sheet on a
// tab first (device, alarms, bluetooth, cameras, radio, theme, security; "off" closes it), ?alarm=new
// opens the alarm editor, ?ring=preview shows the ringing page silently, and ?theme= switches the palette,
// so the sheet and the themes can be looked at without a finger on the device. Off unless the
// Screen web access switch is on: the options change what the device is doing.
func (f *Feature) registerScreen(mux *http.ServeMux) {
	mux.HandleFunc("/screen.png", allowed(screenOpen, func(w http.ResponseWriter, r *http.Request) {
		if theme := r.URL.Query().Get("theme"); theme != "" {
			display.Get().SetTheme(theme)
		}
		switch r.URL.Query().Get("wifi") {
		case "list":
			display.Get().OpenWifi(false)
			time.Sleep(6 * time.Second) // a scan takes a few seconds
		case "keyboard":
			display.Get().OpenWifi(true)
			time.Sleep(700 * time.Millisecond)
		case "off":
			display.Get().CloseWifi()
		}
		if station := r.URL.Query().Get("radio"); station != "" {
			// A station to start (or "stop"), so the now-playing screen can be looked at.
			if station == "stop" {
				home.Get().Stop()
			} else {
				home.Get().Play(station)
			}
		}
		if tab := r.URL.Query().Get("sheet"); tab != "" && display.Get().OpenSheet(tab) {
			time.Sleep(700 * time.Millisecond)
		}
		if r.URL.Query().Get("alarm") == "new" {
			// The alarm editor on a new alarm, so its page can be looked at.
			display.Get().OpenSheet("alarms")
			display.Get().EditNewAlarm()
			time.Sleep(700 * time.Millisecond)
		}
		if r.URL.Query().Get("ring") == "preview" {
			// The ringing page, silent, for a look.
			display.Get().PreviewRing(10 * time.Second)
			time.Sleep(700 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		img, err := display.Get().Screenshot(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		png.Encode(w, img)
	}))
}

func screenOpen() bool { return config.Get().Security.Screen }
