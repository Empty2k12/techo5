//go:build dot

package buttons

import "time"

// longHoldAfter is how long the action button is held to start Bluetooth pairing, which is how the
// stock Echo used a long hold too: there is no screen to ask for it on.
const longHoldAfter = 5 * time.Second

// The Echo Dot 2's four buttons, by evdev key code.
var codes = map[uint16]Name{
	113: Mute,
	114: VolumeDown,
	115: VolumeUp,
	138: Action,
}
