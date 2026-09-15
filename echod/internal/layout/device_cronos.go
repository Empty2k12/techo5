//go:build !dot

package layout

// The Echo Show 5 2nd gen on LineageOS 18.1: nothing of Amazon's is taken over. The daemon is an
// init service of its own (tools/init/techo5.rc) with its state on /data. There are no vendor boot
// hooks to keep current, so AnimationScripts is empty and update.Ensure has nothing to write.
const (
	// The daemon is /system/bin/techo5, which is what tools/init/techo5.rc runs and what the
	// updater replaces in place (remounting / rw, as the Dot does for /system).
	Dir        = "/system/bin"
	BinaryName = "techo5"
	StateDir   = "/data/misc/techo5"

	Service     = Dir + "/" + BinaryName
	ServiceName = "techo5"

	StockLabel = "u:object_r:system_file:s0"

	StartAnimation = ""
	StopAnimation  = ""

	FirewallHook = ""

	// LogTag is the daemon's logcat tag: `adb logcat -s techo5`.
	LogTag = "techo5"

	Manufacturer = "TECHO5"
	Model        = "Echo Show 5 2nd gen (cronos)"
	Board        = "cronos"

	// DefaultName is the fallback display name when a device has none recorded.
	DefaultName = "Echo Show"
)

var AnimationScripts = []string{}
