// Package timezone takes the device's time zone from Home Assistant.
//
// The image carries no zone of its own: /etc/localtime is a link to a file on userdata that boot
// makes UTC when there is none yet. Once Home Assistant is connected the device asks it for the time;
// the answer carries Home Assistant's zone as a POSIX rule ("MST7MDT,M3.2.0,M11.1.0"), or as a zone
// name from a Home Assistant that sends one. A zone that differs from the one in force is written to
// userdata — a zoneinfo file built from the rule, or a link to the named zone — so it holds across
// reboots, slot changes and Home Assistant being away, and is applied to the running daemon at once.
package timezone

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/go-esphome-device/api"
	"google.golang.org/protobuf/proto"

	"github.com/HuskerMinion/techo5/echod/internal/component"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

func init() {
	component.Register(component.Device, Get(), component.Order(11))
}

// Where the zone lives. /etc/localtime and /etc/timezone in the image link to these.
var (
	nameFile = layout.StateDir + "/timezone"
	linkFile = layout.StateDir + "/localtime"
	zoneinfo = "/usr/share/zoneinfo"
)

type Zone struct{}

var shared = &Zone{}

func Get() *Zone { return shared }

func (z *Zone) Name() string { return "time zone" }

// Handle asks for the time once Home Assistant has subscribed, which is the first moment on a
// connection it answers requests, and takes the zone from the answer.
func (z *Zone) Handle(ctx context.Context, c *esphome.Conn, msg proto.Message) error {
	switch m := msg.(type) {
	case *api.SubscribeHomeAssistantStatesRequest:
		if err := c.Send(&api.GetTimeRequest{}); err != nil {
			slog.Debug("asking home assistant for the time", "err", err)
		}
	case *api.GetTimeResponse:
		if err := z.apply(m.GetTimezone()); err != nil {
			slog.Warn("time zone from home assistant not applied", "zone", m.GetTimezone(), "err", err)
		}
	}
	return nil
}

// Current is the zone in force, from userdata; empty when none has been set.
func (z *Zone) Current() string {
	b, err := os.ReadFile(nameFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// apply makes zone the device's zone: a POSIX rule, or a zone name this image has. Nothing is written
// when it is the saved zone already, or when it is neither.
func (z *Zone) apply(zone string) error {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return nil // an older Home Assistant sends no zone
	}
	if zone == z.Current() {
		return nil // in force since boot: /etc/localtime already points at it
	}
	if err := os.MkdirAll(filepath.Dir(nameFile), 0o755); err != nil {
		return err
	}
	tmp := linkFile + ".new"
	_ = os.Remove(tmp)

	var loc *time.Location
	if valid(zone) {
		if _, err := os.Stat(filepath.Join(zoneinfo, zone)); err == nil {
			l, err := time.LoadLocation(zone)
			if err != nil {
				return err
			}
			if err := os.Symlink(filepath.Join(zoneinfo, zone), tmp); err != nil {
				return err
			}
			loc = l
		}
	}
	if loc == nil {
		l, err := loadRule(zone)
		if err != nil {
			return err
		}
		if err := os.WriteFile(tmp, tzif(zone), 0o644); err != nil {
			return err
		}
		loc = l
	}
	// The file is replaced through a temporary one so a reader never finds it missing; the name goes
	// last, as the record that the change is complete.
	if err := os.Rename(tmp, linkFile); err != nil {
		return err
	}
	if err := os.WriteFile(nameFile, []byte(zone+"\n"), 0o644); err != nil {
		return err
	}
	time.Local = loc
	name, offset := time.Now().In(loc).Zone()
	slog.Info("time zone from home assistant", "zone", zone, "now", name, "offset_h", float64(offset)/3600)
	return nil
}

// valid is a plausible IANA name: letters, digits and _+-/ only, no dot segments, so it cannot walk
// out of the zoneinfo directory.
func valid(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '+', r == '/':
		default:
			return false
		}
	}
	return true
}
