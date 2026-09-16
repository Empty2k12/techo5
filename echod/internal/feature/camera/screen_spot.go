//go:build spot

package camera

import (
	"image/png"
	"net/http"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/display"
)

// registerScreen adds /screen.png: what the round panel shows, as last drawn. Off unless the Screen
// web access switch is on.
func (f *Feature) registerScreen(mux *http.ServeMux) {
	mux.HandleFunc("/screen.png", allowed(screenOpen, func(w http.ResponseWriter, r *http.Request) {
		img := display.Get().Screenshot()
		if img == nil {
			http.Error(w, "the screen is not open", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		png.Encode(w, img)
	}))
}

func screenOpen() bool { return config.Get().Security.Screen }
