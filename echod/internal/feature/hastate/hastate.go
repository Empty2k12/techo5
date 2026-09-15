// Package hastate follows entities in Home Assistant. The ESPHome protocol lets a device ask for
// the states of entities it names, and Home Assistant then sends each one whenever it changes —
// which is how the screen knows the weather and the radio's station list without a browser.
//
// What is wanted is declared before a client connects (Want), since Home Assistant asks once per
// connection what the device would like; a change to the list takes a reconnect.
package hastate

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/go-esphome-device/api"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hook"
)

func init() {
	component.Register(component.Device, Get(), component.Order(35))
}

// Key names one thing followed: an entity's state (Attribute empty) or one attribute of it.
type Key struct {
	Entity    string
	Attribute string
}

// Update is one value arriving.
type Update struct {
	Key
	Value string
}

type Tracker struct {
	// Changed fires on every value that arrives; listeners must not block.
	Changed hook.Hook[Update]

	mu     sync.Mutex
	wanted []Key
	values map[Key]string
}

var (
	once   sync.Once
	shared *Tracker
)

func Get() *Tracker {
	once.Do(func() { shared = &Tracker{values: map[Key]string{}} })
	return shared
}

func (t *Tracker) Name() string { return "home assistant states" }

// Want asks for an entity's state, or one attribute of it. Duplicates are fine.
func (t *Tracker) Want(entity, attribute string) {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return
	}
	k := Key{Entity: entity, Attribute: attribute}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, w := range t.wanted {
		if w == k {
			return
		}
	}
	t.wanted = append(t.wanted, k)
}

// Forget drops everything wanted, for a component rebuilding its list.
func (t *Tracker) Forget() {
	t.mu.Lock()
	t.wanted = nil
	t.mu.Unlock()
}

// Value is the last value seen for a key, if any has arrived.
func (t *Tracker) Value(entity, attribute string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.values[Key{Entity: entity, Attribute: attribute}]
	return v, ok
}

// State is the entity's state, empty when unknown.
func (t *Tracker) State(entity string) string {
	v, _ := t.Value(entity, "")
	return v
}

// Handle answers Home Assistant's question about what to follow, and takes the values as they
// come. Other messages are not ours and pass through.
func (t *Tracker) Handle(ctx context.Context, c *esphome.Conn, msg proto.Message) error {
	switch m := msg.(type) {
	case *api.SubscribeHomeAssistantStatesRequest:
		t.mu.Lock()
		wanted := append([]Key(nil), t.wanted...)
		t.mu.Unlock()
		for _, k := range wanted {
			if err := c.Send(&api.SubscribeHomeAssistantStateResponse{EntityId: k.Entity, Attribute: k.Attribute}); err != nil {
				return err
			}
		}
		slog.Info("following home assistant entities", "count", len(wanted))
	case *api.HomeAssistantStateResponse:
		k := Key{Entity: m.EntityId, Attribute: m.Attribute}
		t.mu.Lock()
		old, had := t.values[k]
		t.values[k] = m.State
		t.mu.Unlock()
		if !had || old != m.State {
			t.Changed.Emit(Update{Key: k, Value: m.State})
		}
	}
	return nil
}

// Options parses the string Home Assistant sends for a list attribute — Python's repr of a list,
// like ['101.1 WXYZ', "Kitchen Echo"] — into its items. Anything else comes back as one item.
func Options(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var out []string
	body := s[1 : len(s)-1]
	for i := 0; i < len(body); {
		for i < len(body) && (body[i] == ' ' || body[i] == ',') {
			i++
		}
		if i >= len(body) {
			break
		}
		q := body[i]
		if q != '\'' && q != '"' {
			// Not quoted: up to the next comma.
			j := strings.IndexByte(body[i:], ',')
			if j < 0 {
				j = len(body) - i
			}
			out = append(out, strings.TrimSpace(body[i:i+j]))
			i += j
			continue
		}
		i++
		var b strings.Builder
		for i < len(body) && body[i] != q {
			if body[i] == '\\' && i+1 < len(body) {
				i++
			}
			b.WriteByte(body[i])
			i++
		}
		i++ // closing quote
		out = append(out, b.String())
	}
	return out
}
