//go:build linux && !dot && !spot

package btaudio

import "time"

// autoPick is off on the Echo Show 5: a tap on the screen's list chooses.
const autoPick = false

const pickAfter = 8 * time.Second
