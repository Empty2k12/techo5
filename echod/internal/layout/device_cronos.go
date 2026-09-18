//go:build !dot && !spot

package layout

import (
	"os"
	"strings"
)

// The Echo Show 5 2nd gen on LineageOS 18.1: nothing of Amazon's is taken over. The daemon is an
// init service of its own (tools/init/techo5.rc) with its state on /data. There are no vendor boot
// hooks to keep current, so AnimationScripts is empty and update.Ensure has nothing to write.
const (
	// On Android the daemon is /system/bin/techo5, which is what tools/init/techo5.rc runs and what
	// the updater replaces in place (remounting / rw, as the Dot does for /system). On the Linux image
	// it is /usr/local/bin/techo5 under busybox init (tools/linux/rootfs); see layout.Dir.
	AndroidDir = "/system/bin"
	BinaryName = "techo5"
	StateDir   = "/data/misc/techo5"

	Service     = AndroidDir + "/" + BinaryName
	ServiceName = "techo5"

	StockLabel = "u:object_r:system_file:s0"

	StartAnimation = ""
	StopAnimation  = ""

	FirewallHook = ""

	// LogTag is the daemon's logcat tag: `adb logcat -s techo5`.
	LogTag = "techo5"

	Manufacturer = "TECHO5"

	// DefaultName is the fallback display name when a device has none recorded.
	DefaultName = "Echo Show"
)

var AnimationScripts = []string{}

// Board and Model name which Echo Show 5 this is. One binary serves both generations: they are the
// same board a generation apart, with the same panel, touch controller, capture path, PCM device
// numbers, radio and partitions, so the handful of places that differ (the speaker codec, the mute
// latch, the camera sensor) ask at run time rather than at build time. See docs/porting-checkers.md.
var (
	Board = boardName(readCmdline(), gatingDir)
	Model = map[string]string{
		cronos:   "Echo Show 5 2nd gen (cronos)",
		checkers: "Echo Show 5 1st gen (checkers)",
	}[Board]
)

const (
	cronos   = "cronos"
	checkers = "checkers"

	// gatingDir is the 1st gen's mute driver (drivers/misc/gating.c); the 2nd gen has gpio-privacy
	// there instead, which is why its presence identifies the board when the command line cannot.
	gatingDir = "/sys/devices/platform/amazon-gating"
)

// Checkers reports whether this is the 1st gen (2019).
func Checkers() bool { return Board == checkers }

// boardName works the generation out from the kernel command line, where the bootloader names the
// panel driver it is built for (lcm=1-st7701s_wsvga_dsi_vdo_cronos_st_truly). Off a device, or on a
// command line that names neither, the mute driver decides: only the 1st gen has amazon-gating.
func boardName(cmdline, gating string) string {
	switch {
	case strings.Contains(cmdline, checkers):
		return checkers
	case strings.Contains(cmdline, cronos):
		return cronos
	}
	if _, err := os.Stat(gating); err == nil {
		return checkers
	}
	return cronos
}

func readCmdline() string {
	b, _ := os.ReadFile("/proc/cmdline")
	return string(b)
}
