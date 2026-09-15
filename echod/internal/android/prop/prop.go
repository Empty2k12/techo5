// Package prop reads and sets Android system properties through the platform's own getprop and
// setprop, by full path since the daemon's PATH is init's.
//
// EchoLocal spoke init's property_service socket protocol directly, which is the Android 5.1 wire
// format and wrong on anything newer; setprop speaks whichever protocol the platform has.
package prop

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	getprop = "/system/bin/getprop"
	setprop = "/system/bin/setprop"

	// callTimeout bounds one call: setprop waits for init to act, and init can be busy.
	callTimeout = 5 * time.Second
)

// Set assigns a system property. init applies its own permission checks, so a rejected
// write reports no error here — verify the effect, not the call.
func Set(name, value string) error {
	cmd := exec.Command(setprop, name, value)
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("prop: setting %s: %w", name, err)
		}
		return nil
	case <-time.After(callTimeout):
		_ = cmd.Process.Kill()
		return fmt.Errorf("prop: setting %s: timed out", name)
	}
}

// Get reads a system property. An unset property reads as empty with no error, which is what
// getprop reports for one.
func Get(name string) (string, error) {
	out, err := exec.Command(getprop, name).Output()
	if err != nil {
		return "", fmt.Errorf("prop: reading %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Stop asks init to stop a service.
func Stop(service string) error { return Set("ctl.stop", service) }

// Start asks init to start a service.
func Start(service string) error { return Set("ctl.start", service) }
