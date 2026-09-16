package ble

// Scanner is what the Bluetooth proxy drives: something that hears advertisements. The Dot's is
// the raw HCI Radio; the Show's goes through BlueZ (bluez.go), since bluetoothd owns the node.
type Scanner interface {
	// Start begins scanning (and advertising, where the scanner can) and calls found for every
	// advertisement heard, on the scanner's own goroutine.
	Start(scan, active bool, advertisement []byte, found func(Advertisement)) error
	Stop()
	Running() bool
	Scanning() bool
	Reports() uint64
}

var _ Scanner = (*Radio)(nil)
