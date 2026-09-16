//go:build dot

package ble

// Default is the scanner for this device: the raw HCI radio.
func Default() Scanner { return Get() }
