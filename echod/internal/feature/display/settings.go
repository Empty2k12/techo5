//go:build !dot

package display

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/feature/media"
	"github.com/HuskerMinion/techo5/echod/internal/feature/mute"
	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// gather collects what the settings sheet shows. Cheap enough per frame: a few reads and one
// interface listing.
func (d *Display) gather(s scene, restartArm time.Time, tab int) settings {
	d.mu.Lock()
	st := settings{tab: tab, page: d.page, brightness: d.ceiling, auto: d.autoOn, now: s.now, restartArm: restartArm}
	d.mu.Unlock()
	if st.brightness == 0 {
		st.brightness = config.DefaultScreenBrightness
	}
	st.muted, _ = mute.Get().Muted()
	st.volume = media.Get().Volume()

	c := config.Get()
	st.name = c.Device.Name
	if st.name == "" {
		st.name = "TECHO5"
	}
	st.wakeWord = strings.ReplaceAll(c.Wake.Slot(0).ID, "_", " ")
	if st.wakeWord == "" {
		st.wakeWord = "off"
	}
	st.version = layout.Version
	st.theme = themes[themeIndex(config.Get().Screen.Theme)].name
	st.slot = slotName()
	st.address = address()

	return st
}

// slotName is which rootfs slot booted, from the file the initramfs leaves; empty on Android.
func slotName() string {
	b, err := os.ReadFile("/run/techo5/slot")
	if err != nil {
		return "-"
	}
	return strings.TrimSpace(string(b))
}

// address is the device's IPv4 address, which is how people find it on the network.
func address() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "-"
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 || i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				return ipn.IP.String()
			}
		}
	}
	return "-"
}

// restart reboots the device the plain way; the slot store and the daemon's state are on disk
// already, so nothing needs saying goodbye to.
func restart() {
	syscall.Sync()
	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		slog.Error("restart failed", "err", err)
	}
}
