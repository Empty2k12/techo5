//go:build dot

package camera

import "net/http"

// registerScreen does nothing on the Dot, which has no panel.
func (f *Feature) registerScreen(*http.ServeMux) {}
