//go:build !dot

package privacy

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// The Echo Show 5 exposes its mute through the gpio-privacy platform driver rather than a bare
// GPIO: `state` reads the latch (1 while the microphones are cut) and a write to `enable` pulses
// the enable line for the device tree's toggle duration (1000 ms), which flips it. The red mute
// indicator is part of the same circuit and follows the latch on its own.
const (
	dir    = "/sys/devices/platform/gpio-privacy"
	state  = dir + "/state"
	enable = dir + "/enable"

	// toggleLag is how long the latch takes to report the flip once enable has been pulsed,
	// with margin over the 1000 ms pulse.
	toggleLag = 1400 * time.Millisecond
)

// platform is the mute as the gpio-privacy driver exposes it.
type platform struct{}

func (platform) Get() (bool, error) { return reads(state, "1") }

// The button feeds the same latch, so by the time the key event arrives the state has already
// changed. Acting on the press would toggle it straight back.
func (platform) HardwareToggles() bool { return true }

func (platform) Lag() time.Duration { return toggleLag }

// Set flips the latch when it disagrees with muted and waits for it to report the change.
func (p platform) Set(muted bool) error {
	is, err := p.Get()
	if err != nil {
		return err
	}
	if is == muted {
		return nil
	}
	if err := write(enable, "1"); err != nil {
		return fmt.Errorf("privacy: pulsing enable: %w", err)
	}
	deadline := time.Now().Add(toggleLag)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if now, err := p.Get(); err == nil && now == muted {
			return nil
		}
	}
	return errors.New("privacy: the mute latch did not change")
}

func (p platform) Toggle() (bool, error) {
	is, err := p.Get()
	if err != nil {
		return false, err
	}
	return !is, p.Set(!is)
}

// noLED stands in for the mute button's light: on this device it is driven by the privacy circuit
// itself and has no brightness control, so it always reads bright while muted.
type noLED struct{}

func (noLED) SetBright(bool) error { return nil }
func (noLED) Bright() (bool, error) { return true, nil }

func platformMute() (Mute, error) {
	if !present(state) {
		return nil, fmt.Errorf("privacy: %s is missing", state)
	}
	return platform{}, nil
}

func platformLight() (LED, error) { return noLED{}, nil }

func write(path, value string) error { return os.WriteFile(path, []byte(value), 0o644) }

func reads(path, value string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(b)) == value, nil
}
