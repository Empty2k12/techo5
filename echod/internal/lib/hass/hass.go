// Package hass is Home Assistant's REST API, for what the ESPHome link cannot carry: a forecast
// (a service that answers), pictures, cameras. It needs a long-lived token, handed to the device
// once through an action and kept on userdata.
package hass

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// Path is where the URL and token live: next to the PSK, readable by root only.
var Path = filepath.Join(filepath.Dir(layout.KeyPath), "hass.json")

type access struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Client struct {
	mu   sync.Mutex
	acc  access
	http *http.Client
}

var (
	once   sync.Once
	shared *Client
)

// Get is the client; it reads the saved access the first time.
func Get() *Client {
	once.Do(func() {
		shared = &Client{http: &http.Client{Timeout: 15 * time.Second}}
		if b, err := os.ReadFile(Path); err == nil {
			_ = json.Unmarshal(b, &shared.acc)
		}
	})
	return shared
}

// Set stores the URL (like http://192.168.1.20:8123) and token.
func (c *Client) Set(url, token string) error {
	url = strings.TrimRight(strings.TrimSpace(url), "/")
	token = strings.TrimSpace(token)
	if url == "" || token == "" {
		return errors.New("hass: url and token are both needed")
	}
	c.mu.Lock()
	c.acc = access{URL: url, Token: token}
	c.mu.Unlock()
	b, _ := json.Marshal(c.acc)
	if err := os.MkdirAll(filepath.Dir(Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(Path, b, 0o600)
}

// Ready reports whether there is an access to use.
func (c *Client) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acc.URL != "" && c.acc.Token != ""
}

func (c *Client) do(method, path string, body any) ([]byte, error) {
	c.mu.Lock()
	acc := c.acc
	c.mu.Unlock()
	if acc.URL == "" || acc.Token == "" {
		return nil, errors.New("hass: no access configured")
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, acc.URL+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+acc.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("hass: %s %s: %s", method, path, resp.Status)
	}
	return out, nil
}

// Day is one day of a forecast.
type Day struct {
	When      time.Time
	Condition string
	High      float64
	Low       float64
	Rain      int // precipitation probability, percent, -1 when not given
}

// Forecast is the daily forecast for a weather entity, as many days as it gives.
func (c *Client) Forecast(entity string) ([]Day, error) {
	out, err := c.do("POST", "/api/services/weather/get_forecasts?return_response",
		map[string]any{"entity_id": entity, "type": "daily"})
	if err != nil {
		return nil, err
	}
	var resp struct {
		ServiceResponse map[string]struct {
			Forecast []struct {
				Datetime      string   `json:"datetime"`
				Condition     string   `json:"condition"`
				Temperature   *float64 `json:"temperature"`
				Templow       *float64 `json:"templow"`
				Precipitation *float64 `json:"precipitation_probability"`
			} `json:"forecast"`
		} `json:"service_response"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	fc, ok := resp.ServiceResponse[entity]
	if !ok {
		return nil, fmt.Errorf("hass: no forecast for %s", entity)
	}
	days := make([]Day, 0, len(fc.Forecast))
	for _, f := range fc.Forecast {
		d := Day{Condition: f.Condition, Rain: -1}
		if t, err := time.Parse(time.RFC3339, f.Datetime); err == nil {
			d.When = t.Local()
		}
		if f.Temperature != nil {
			d.High = *f.Temperature
		}
		if f.Templow != nil {
			d.Low = *f.Templow
		}
		if f.Precipitation != nil {
			d.Rain = int(*f.Precipitation)
		}
		days = append(days, d)
	}
	return days, nil
}

// State is an entity's state and attributes.
type State struct {
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

func (c *Client) State(entity string) (State, error) {
	out, err := c.do("GET", "/api/states/"+entity, nil)
	if err != nil {
		return State{}, err
	}
	var s State
	return s, json.Unmarshal(out, &s)
}

// Fetch gets bytes from a path on Home Assistant (an entity_picture, a camera_proxy image).
func (c *Client) Fetch(path string) ([]byte, error) {
	return c.do("GET", path, nil)
}
