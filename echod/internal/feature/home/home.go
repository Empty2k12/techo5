// Package home is the house on the screen: the weather on the clock, and a radio page that lists
// the stations Home Assistant knows and plays one through Home Assistant's own script — the same
// path the old dashboard's chips used, so favourites, search and the station finder stay where
// they are. Nothing here is baked in: Home Assistant tells the device which entities to follow and
// which script to call, through two actions (esphome.<device>_home_weather and _home_radio).
package home

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/hastate"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hook"
)

func init() {
	component.Register(component.Device, Get(), component.Order(36))
}

// Weather is what the clock shows.
type Weather struct {
	Condition string // Home Assistant's state: "partlycloudy", "rain"…
	Temp      string // "75°" already formatted, empty when unknown
}

// Radio is what the radio page shows.
type Radio struct {
	Configured bool
	Stations   []string
	Now        string // the station Home Assistant says is playing, empty for none
	Playing    bool   // the device's own player is running
	Chosen     string // the station tapped last, until Now catches up
}

type Feature struct {
	// Changed fires when anything shown changes; listeners must not block.
	Changed hook.Hook[struct{}]

	mu       sync.Mutex
	chosen   string
	url      string // the stream playing, from the media player
	urlName  string // its station name once found
	forecast []hass.Day
	fetched  time.Time
	poke     chan struct{}
}

// forecastEvery is how often the forecast is refreshed while there is a weather entity.
const forecastEvery = 30 * time.Minute

// Run keeps the forecast current. Nothing to do without a token or a weather entity.
func (f *Feature) Run(ctx context.Context) error {
	for {
		f.refreshForecast()
		select {
		case <-ctx.Done():
			return nil
		case <-f.poke:
		case <-time.After(forecastEvery):
		}
	}
}

func (f *Feature) refreshForecast() {
	entity := config.Get().Home.Weather
	if entity == "" || !hass.Get().Ready() {
		return
	}
	days, err := hass.Get().Forecast(entity)
	if err != nil {
		slog.Warn("home: forecast", "err", err)
		return
	}
	f.mu.Lock()
	f.forecast, f.fetched = days, time.Now()
	f.mu.Unlock()
	f.Changed.Emit(struct{}{})
}

// Forecast is the daily forecast as last fetched, today first.
func (f *Feature) Forecast() []hass.Day {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hass.Day(nil), f.forecast...)
}

var (
	once   sync.Once
	shared *Feature
)

func Get() *Feature {
	once.Do(func() {
		shared = &Feature{poke: make(chan struct{}, 1)}
		hastate.Get().Changed.Listen(func(hastate.Update) { shared.Changed.Emit(struct{}{}) })
		media.Get().OnPlay.Listen(shared.played)
	})
	return shared
}

// played is a new stream starting: whatever was tapped is no longer the answer to "what is
// this", so its name is looked up from the URL.
func (f *Feature) played(url string) {
	f.mu.Lock()
	f.url, f.urlName, f.chosen = url, "", ""
	f.mu.Unlock()
	f.Changed.Emit(struct{}{})
	go f.nameStream(url)
}

// nameStream finds a station name for a stream URL in the lists Home Assistant keeps —
// favourites and the last search — and falls back to the stream's host.
func (f *Feature) nameStream(url string) {
	name := ""
	if hass.Get().Ready() {
		for _, entity := range []string{"sensor.radio_favorites", "sensor.radio_search_results"} {
			st, err := hass.Get().State(entity)
			if err != nil {
				continue
			}
			for _, key := range []string{"favorites", "results"} {
				items, _ := st.Attributes[key].([]any)
				for _, it := range items {
					m, _ := it.(map[string]any)
					if u, _ := m["url"].(string); u == url {
						if n, _ := m["name"].(string); n != "" {
							name = n
						}
					}
				}
			}
			if name != "" {
				break
			}
		}
	}
	// No host fallback: Home Assistant proxies streams through itself, so the host would be its
	// own address, which is not a station. Unknown stays unknown and the page falls back to the
	// "last station" text or what was tapped.
	f.mu.Lock()
	if f.url == url {
		f.urlName = name
	}
	f.mu.Unlock()
	f.Changed.Emit(struct{}{})
}

func (f *Feature) wake() {
	select {
	case f.poke <- struct{}{}:
	default:
	}
}

func (f *Feature) Name() string { return "home" }

// Restore registers what to follow from the saved configuration, before Home Assistant connects.
func (f *Feature) Restore(c config.Config) { f.want(c.Home) }

func (f *Feature) want(h config.Home) {
	t := hastate.Get()
	t.Forget()
	if h.Weather != "" {
		t.Want(h.Weather, "")
		t.Want(h.Weather, "temperature")
		t.Want(h.Weather, "temperature_unit")
	}
	for _, s := range h.Radio.Stations {
		t.Want(s, "options")
	}
	if h.Radio.Now != "" {
		t.Want(h.Radio.Now, "")
	}
}

// Actions are how Home Assistant configures this: which weather entity to show, and how the
// radio page is wired. Both persist and take effect at the next connection.
func (f *Feature) Actions() []*esphome.Action {
	return []*esphome.Action{
		f.accessAction(),
		{
			Name: "home_weather",
			Args: []esphome.Arg{{Name: "entity", Type: esphome.ArgString}},
			Run: func(c esphome.Call) (any, error) {
				entity := strings.TrimSpace(c.String("entity"))
				if err := config.Set().Home().Weather(entity); err != nil {
					return nil, err
				}
				slog.Info("home: weather entity set", "entity", entity)
				f.rewire()
				return nil, nil
			},
		},
		{
			Name: "home_radio",
			Args: []esphome.Arg{
				{Name: "stations", Type: esphome.ArgString}, // input_select entities, comma separated
				{Name: "now", Type: esphome.ArgString},      // entity whose state names the playing station
				{Name: "service", Type: esphome.ArgString},  // script that plays a station
				{Name: "field", Type: esphome.ArgString},    // its station argument (default "station")
				{Name: "speaker_field", Type: esphome.ArgString},
				{Name: "speaker", Type: esphome.ArgString}, // this device's media_player entity
			},
			Run: func(c esphome.Call) (any, error) {
				r := config.Radio{
					Now:          strings.TrimSpace(c.String("now")),
					Service:      strings.TrimSpace(c.String("service")),
					Field:        strings.TrimSpace(c.String("field")),
					SpeakerField: strings.TrimSpace(c.String("speaker_field")),
					Speaker:      strings.TrimSpace(c.String("speaker")),
				}
				for _, s := range strings.Split(c.String("stations"), ",") {
					if s = strings.TrimSpace(s); s != "" {
						r.Stations = append(r.Stations, s)
					}
				}
				if r.Field == "" {
					r.Field = "station"
				}
				if r.SpeakerField == "" {
					r.SpeakerField = "speaker"
				}
				if err := config.Set().Home().Radio(r); err != nil {
					return nil, err
				}
				slog.Info("home: radio wired", "stations", r.Stations, "service", r.Service, "now", r.Now)
				f.rewire()
				return nil, nil
			},
		},
	}
}

// rewire re-registers what to follow and asks for a reconnect, since Home Assistant only asks
// what the device wants once per connection.
func (f *Feature) rewire() {
	f.want(config.Get().Home)
	component.Reconnect.Emit(struct{}{})
	f.wake()
	f.Changed.Emit(struct{}{})
}

// accessAction is the third action: the URL and a long-lived token for Home Assistant's REST
// API, for the forecast and, later, pictures and cameras.
func (f *Feature) accessAction() *esphome.Action {
	return &esphome.Action{
		Name: "home_assistant",
		Args: []esphome.Arg{{Name: "url", Type: esphome.ArgString}, {Name: "token", Type: esphome.ArgString}},
		Run: func(c esphome.Call) (any, error) {
			if err := hass.Get().Set(c.String("url"), c.String("token")); err != nil {
				return nil, err
			}
			slog.Info("home: home assistant access stored")
			f.wake()
			return nil, nil
		},
	}
}

// Weather is the current reading for the clock.
func (f *Feature) Weather() Weather {
	h := config.Get().Home
	if h.Weather == "" {
		return Weather{}
	}
	t := hastate.Get()
	w := Weather{Condition: t.State(h.Weather)}
	if temp, ok := t.Value(h.Weather, "temperature"); ok && temp != "" && temp != "None" {
		if i := strings.IndexByte(temp, '.'); i > 0 {
			temp = temp[:i]
		}
		w.Temp = temp + "°"
	}
	return w
}

// Radio is the page's content.
func (f *Feature) Radio() Radio {
	h := config.Get().Home.Radio
	r := Radio{Configured: h.Configured()}
	if !r.Configured {
		return r
	}
	t := hastate.Get()
	seen := map[string]bool{}
	for _, entity := range h.Stations {
		v, ok := t.Value(entity, "options")
		if !ok {
			continue
		}
		for _, name := range hastate.Options(v) {
			name = strings.TrimSpace(name)
			if name == "" || seen[name] || strings.HasPrefix(strings.ToLower(name), "select a") {
				continue
			}
			seen[name] = true
			r.Stations = append(r.Stations, name)
		}
	}
	r.Playing, _ = media.Get().Playing()
	f.mu.Lock()
	r.Chosen = f.chosen
	r.Now = f.urlName
	f.mu.Unlock()
	// The stream's own name wins; Home Assistant's "last station" text is the fallback.
	if r.Now == "" && h.Now != "" {
		r.Now = t.State(h.Now)
		if r.Now == "unknown" || r.Now == "unavailable" {
			r.Now = ""
		}
	}
	return r
}

// Play asks Home Assistant to play a station on this device.
func (f *Feature) Play(station string) {
	h := config.Get().Home.Radio
	if !h.Configured() {
		return
	}
	speaker := h.Speaker
	if speaker == "" {
		speaker = "media_player." + layout.Slug(config.Get().Device.Name) + "_speaker"
	}
	f.mu.Lock()
	f.chosen = station
	f.mu.Unlock()
	component.CallService.Emit(component.Call{
		Service: h.Service,
		Data:    map[string]string{h.Field: station, h.SpeakerField: speaker},
	})
	f.Changed.Emit(struct{}{})
}

// Stop ends whatever the player is doing.
func (f *Feature) Stop() {
	media.Get().Pause()
	f.mu.Lock()
	f.chosen = ""
	f.mu.Unlock()
	f.Changed.Emit(struct{}{})
}
