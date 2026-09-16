//go:build !dot

package config

// DefaultDuckOnNearMiss: Off on the Echo Show 5 until it has been tried there: the Show tunes its wake word over playback
// its own way, and this leaves it as it was.
const DefaultDuckOnNearMiss = false
