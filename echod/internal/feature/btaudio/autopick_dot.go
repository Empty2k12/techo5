//go:build linux && dot

package btaudio

import "time"

// autoPick: the Echo Dot has no screen to choose a device on, so pairing mode (the switch in Home
// Assistant) guesses. A phone that pairs with the device will play to it; if none does, the strongest
// audio device in pairing mode that it hears is connected to (and the next if that fails), until one
// connects or pairing mode ends.
const autoPick = true

// pickAfter lets the scan hear everything nearby first, so the strongest is not simply the first, and
// gives a phone time to pair with the device instead: a pairing coming in cancels the pick
// (btaudio.go, incoming).
const pickAfter = 20 * time.Second
