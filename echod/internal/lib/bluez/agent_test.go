//go:build linux

package bluez

import "testing"

// Pairing is answered only in pairing mode; connections only for the audio profiles.
func TestAgentRules(t *testing.T) {
	on := false
	g := agent{pairing: func() bool { return on }}

	if g.RequestConfirmation("/x", 123456) == nil || g.RequestAuthorization("/x") == nil {
		t.Error("a pairing was accepted outside pairing mode")
	}
	if _, err := g.RequestPinCode("/x"); err == nil {
		t.Error("a PIN was given outside pairing mode")
	}
	on = true
	if g.RequestConfirmation("/x", 123456) != nil || g.RequestAuthorization("/x") != nil {
		t.Error("a pairing was refused in pairing mode")
	}

	for uuid, want := range map[string]bool{
		"0000110B-0000-1000-8000-00805F9B34FB": true,  // Audio Sink, upper case
		"0000110e-0000-1000-8000-00805f9b34fb": true,  // AVRCP
		"00001124-0000-1000-8000-00805f9b34fb": false, // HID
		"00001116-0000-1000-8000-00805f9b34fb": false, // PAN NAP
		"00001101-0000-1000-8000-00805f9b34fb": false, // Serial Port
	} {
		if got := g.AuthorizeService("/x", uuid) == nil; got != want {
			t.Errorf("%s allowed=%v, want %v", uuid, got, want)
		}
	}
}
