package config

// Bluetooth is whether the device also acts as a BLE proxy for Home Assistant, and which audio
// device (earbuds, a speaker) it plays to when that device is around.
type Bluetooth struct {
	Proxy bool `json:"proxy"`

	// Audio is the last audio device paired from this device: its address and the name it gave,
	// so the daemon can try it again after a reboot and the screen can say what it is waiting for.
	Audio     string `json:"audio,omitempty"`
	AudioName string `json:"audio_name,omitempty"`
}

// Off unless asked for: it keeps a radio scanning that shares an antenna with wifi.
const DefaultBluetoothProxy = false

func defaultBluetooth() Bluetooth {
	return Bluetooth{Proxy: DefaultBluetoothProxy}
}

type BluetoothWriter struct{ st *Store }

func (w BluetoothWriter) Proxy(v bool) error {
	return w.st.Update(func(c *Config) { c.Bluetooth.Proxy = v })
}

// Audio remembers the audio device; empty forgets it.
func (w BluetoothWriter) Audio(address, name string) error {
	return w.st.Update(func(c *Config) { c.Bluetooth.Audio, c.Bluetooth.AudioName = address, name })
}
