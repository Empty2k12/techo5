//go:build !linux

package wifi

import "os"

var sigRenew = os.Interrupt
