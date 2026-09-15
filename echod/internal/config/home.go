package config

// Home is what the device shows and reaches for in Home Assistant beyond its own entities: a
// weather entity for the clock screen, and the radio — the selects whose options are the
// stations, the text that names what is playing, and the script that plays one. All of it is
// set from Home Assistant through the device's actions, so nothing here is baked in.
type Home struct {
	// Weather is a weather.* entity shown on the idle screen; empty shows none.
	Weather string `json:"weather,omitempty"`

	Radio Radio `json:"radio"`

	// Cameras are camera.* entities and the names to say for them, in the order the list shows.
	Cameras []Camera `json:"cameras,omitempty"`
}

// Camera is one camera on the screen's list.
type Camera struct {
	Entity string `json:"entity"`
	Name   string `json:"name"`
}

// Radio is how the screen's radio page is wired to the house's own radio setup.
type Radio struct {
	// Stations are input_select entities whose options are station names, listed in order.
	Stations []string `json:"stations,omitempty"`

	// Now is an entity whose state names the station playing, shown while the player runs.
	Now string `json:"now,omitempty"`

	// Service is the script that plays a station, called with Field = station name and
	// SpeakerField = Speaker (this device's media player entity in Home Assistant).
	Service      string `json:"service,omitempty"`
	Field        string `json:"field,omitempty"`
	SpeakerField string `json:"speaker_field,omitempty"`
	Speaker      string `json:"speaker,omitempty"`
}

func defaultHome() Home {
	return Home{Radio: Radio{Field: "station", SpeakerField: "speaker"}}
}

// Configured reports whether the radio page has anything to work with.
func (r Radio) Configured() bool { return len(r.Stations) > 0 && r.Service != "" }

type HomeWriter struct{ st *Store }

func (w HomeWriter) Weather(entity string) error {
	return w.st.Update(func(c *Config) { c.Home.Weather = entity })
}

func (w HomeWriter) Radio(r Radio) error {
	return w.st.Update(func(c *Config) { c.Home.Radio = r })
}

func (w HomeWriter) Cameras(cams []Camera) error {
	return w.st.Update(func(c *Config) { c.Home.Cameras = cams })
}
