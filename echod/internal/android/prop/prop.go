// Package prop reads and sets Android system properties through the platform's own getprop and
// setprop, by full path since the daemon's PATH is init's.
//
// EchoLocal spoke init's property_service socket protocol directly, which is the Android 5.1 wire
// format and wrong on anything newer; setprop speaks whichever protocol the platform has.
//
// Off Android there is no property service. On the Linux image the same names live as files under a
// tmpfs directory, which gives them the one quality the updater's trial marker depends on: they
// survive the supervisor restarting the daemon and are forgotten at reboot. Anywhere that directory
// cannot be made — a workstation running tests — the store is simply absent: writes succeed and do
// nothing, reads come back empty.
package prop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

const (
	getprop = "/system/bin/getprop"
	setprop = "/system/bin/setprop"

	// callTimeout bounds one call: setprop waits for init to act, and init can be busy.
	callTimeout = 5 * time.Second

	// fileStore is where the Linux image keeps properties: one file per name on /run, a tmpfs the
	// initramfs mounts before the daemon exists and nothing writes to disk.
	fileStore = "/run/techo5/prop"
)

// Set assigns a system property. init applies its own permission checks, so a rejected
// write reports no error here — verify the effect, not the call. Off Android it goes to the file
// store, or nowhere when there is none.
func Set(name, value string) error {
	if !layout.OnAndroid() {
		return setFile(name, value)
	}
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
	if !layout.OnAndroid() {
		return getFile(name)
	}
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

// setFile writes a property into the file store. An empty value clears it, as setprop does. A store
// that cannot be created is not an error: there is nothing to keep it in, and nothing asked for one.
func setFile(name, value string) error {
	if err := os.MkdirAll(fileStore, 0o755); err != nil {
		return nil
	}
	path := filepath.Join(fileStore, name)
	if value == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("prop: clearing %s: %w", name, err)
		}
		return nil
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("prop: setting %s: %w", name, err)
	}
	return nil
}

// getFile reads a property from the file store; unset, or no store, reads as empty.
func getFile(name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(fileStore, name))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("prop: reading %s: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}
