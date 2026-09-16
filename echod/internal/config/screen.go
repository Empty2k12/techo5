package config

// Screen is the panel: whether it is lit, how brightly in percent, and whether the room's light
// is allowed to dim it below that. Only the Echo Show has one; on the Dot nothing reads this.
type Screen struct {
	On         bool `json:"on"`
	Brightness int  `json:"brightness"`
	Auto       bool `json:"auto"`

	// Theme names the screen's palette; empty is the first one, "Custom" is Palette.
	Theme   string  `json:"theme,omitempty"`
	Palette Palette `json:"palette,omitempty"`
}

// Palette is a custom theme's five colours, as #rrggbb.
type Palette struct {
	Ground string `json:"ground,omitempty"`
	Accent string `json:"accent,omitempty"`
	Text   string `json:"text,omitempty"`
	Dim    string `json:"dim,omitempty"`
	Rules  string `json:"rules,omitempty"`
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

// Custom saves a palette and makes it the theme.
func (w ScreenWriter) Custom(p Palette) error {
	return w.st.Update(func(c *Config) { c.Screen.Theme, c.Screen.Palette = "Custom", p })
}
