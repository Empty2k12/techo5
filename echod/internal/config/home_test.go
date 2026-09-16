package config

import "testing"

func TestWeatherEntity(t *testing.T) {
	for _, c := range []struct{ saved, want string }{
		{"", DefaultWeather},
		{WeatherOff, ""},
		{"weather.backyard", "weather.backyard"},
	} {
		if got := (Home{Weather: c.saved}).WeatherEntity(); got != c.want {
			t.Errorf("WeatherEntity(%q) = %q, want %q", c.saved, got, c.want)
		}
	}
}
