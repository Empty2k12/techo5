//go:build linux

package wifi

import "syscall"

// sigRenew makes udhcpc ask for a lease again.
var sigRenew = syscall.SIGUSR1
