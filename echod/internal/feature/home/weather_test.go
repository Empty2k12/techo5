package home

import (
	"slices"
	"testing"

	"github.com/HuskerMinion/techo5/echod/internal/config"
)

func TestWeatherOptions(t *testing.T) {
	h := config.Home{Weather: "weather.station", WeatherSources: []string{"weather.forecast_home", "weather.other", "weather.station"}}
	want := []string{weatherNone, config.DefaultWeather, "weather.station", "weather.other"}
	if got := weatherOptions(h); !slices.Equal(got, want) {
		t.Errorf("options = %v, want %v", got, want)
	}
	if got := weatherOptions(config.Home{}); !slices.Equal(got, []string{weatherNone, config.DefaultWeather}) {
		t.Errorf("a new device offers %v", got)
	}
	if chosenOption(config.Home{Weather: config.WeatherOff}) != weatherNone || optionEntity(weatherNone) != "" {
		t.Error("none does not round-trip")
	}
	if chosenOption(config.Home{}) != config.DefaultWeather {
		t.Error("a new device does not show Home Assistant's forecast")
	}
}
