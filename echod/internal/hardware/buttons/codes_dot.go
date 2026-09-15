//go:build dot

package buttons

// The Echo Dot 2's four buttons, by evdev key code.
var codes = map[uint16]Name{
	113: Mute,
	114: VolumeDown,
	115: VolumeUp,
	138: Action,
}
