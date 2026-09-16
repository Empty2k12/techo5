package config

// Screen is the panel: whether it is lit, how brightly in percent, and whether the room's light
// is allowed to dim it below that. Only the Echo Show has one; on the Dot nothing reads this.
type Screen struct {
	On         bool `json:"on"`
	Brightness int  `json:"brightness"`
	Auto       bool `json:"auto"`

	// Theme names the screen's palette; empty is the first one.
	Theme string `json:"theme,omitempty"`
}

// DefaultScreenBrightness is comfortable on a desk in a lit room; the panel's own top is glaring.
const DefaultScreenBrightness = 60

func defaultScreen() Screen {
	return Screen{On: true, Brightness: DefaultScreenBrightness, Auto: true}
}

type ScreenWriter struct{ st *Store }

func (w ScreenWriter) On(v bool) error {
	return w.st.Update(func(c *Config) { c.Screen.On = v })
}

func (w ScreenWriter) Brightness(v int) error {
	return w.st.Update(func(c *Config) { c.Screen.Brightness = v })
}

func (w ScreenWriter) Auto(v bool) error {
	return w.st.Update(func(c *Config) { c.Screen.Auto = v })
}

func (w ScreenWriter) Theme(v string) error {
	return w.st.Update(func(c *Config) { c.Screen.Theme = v })
}
