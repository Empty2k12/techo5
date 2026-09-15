//go:build !dot

package layout

// The Echo Show 5 2nd gen on LineageOS 18.1: nothing of Amazon's is taken over, so the daemon lives
// on /data and is started by an init service of its own (see docs/porting-plan.md). There are no
// vendor boot hooks to keep current, so AnimationScripts is empty and update.Ensure has nothing to
// write.
const (
	Dir      = "/data/techo5"
	StateDir = "/data/misc/techo5"

	Service     = Dir + "/echod"
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
