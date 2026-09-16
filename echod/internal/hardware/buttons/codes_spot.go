//go:build spot

package buttons

// The Echo Spot's three buttons, by evdev key code. Volume comes from gpio-keys (`keys`); the mute
// button is the keypad's power key (`mtk-kpd`), 116. On LineageOS's kernel that key only reports a
// press: it does not cut the microphones (docs/hardware.md in techo5-spot, "Mute button").
var codes = map[uint16]Name{
	114: VolumeDown,
	115: VolumeUp,
	116: Mute,
}
