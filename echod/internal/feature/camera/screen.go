//go:build !dot

package camera

import (
	"context"
	"image/png"
	"net/http"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/display"
)

// registerScreen adds /screen.png: what the panel shows. ?sheet= opens the settings sheet on a
// tab first (device, bluetooth, cameras, radio; "off" closes it) and ?theme= switches the palette,
// so the sheet and the themes can be looked at without a finger on the device.
func (f *Feature) registerScreen(mux *http.ServeMux) {
	mux.HandleFunc("/screen.png", func(w http.ResponseWriter, r *http.Request) {
		if theme := r.URL.Query().Get("theme"); theme != "" {
			display.Get().SetTheme(theme)
		}
		if tab := r.URL.Query().Get("sheet"); tab != "" {
			tabs := map[string]int{"device": 0, "bluetooth": 1, "cameras": 2, "radio": 3, "theme": 4, "off": -1}
			if t, ok := tabs[tab]; ok {
				display.Get().OpenSheet(t)
				time.Sleep(700 * time.Millisecond)
			}
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
	})
}
