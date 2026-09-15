//go:build !dot

package setup

// On LineageOS there is no Amazon userspace to stand down and no vendor firewall to fight, so the
// Echo Show 5 needs nothing of its own on boot beyond what every device gets.
var deviceActions = []Action{}

var deviceLate = []Action{}
