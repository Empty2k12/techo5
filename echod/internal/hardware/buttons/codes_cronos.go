//go:build !dot && !spot

package buttons

// longHoldAfter is zero: the Show's screen is where pairing and everything else is asked for.
const longHoldAfter = 0

// The Echo Show 5's three buttons, by evdev key code. Volume comes from gpio-keys; the mute button
// is its own input device, gpio-privacy-button, which the kernel reports as KEY_POWER (116). There
// is no action button.
var codes = map[uint16]Name{
	114: VolumeDown,
	115: VolumeUp,
	116: Mute,
}
