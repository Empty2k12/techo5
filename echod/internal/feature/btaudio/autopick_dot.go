//go:build linux && (dot || spot)

package btaudio

import "time"

// autoPick: the Echo Dot has no screen to choose a device on, so pairing mode (the switch in Home
// Assistant) connects the strongest audio device in pairing mode that it hears, and the next one if
// that fails, until one connects or pairing mode ends.
const autoPick = true

// pickAfter lets the scan hear everything nearby first, so the strongest is not simply the first.
const pickAfter = 8 * time.Second
