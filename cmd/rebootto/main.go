//go:build linux

// rebootto reboots with a MediaTek boot-mode argument, the way Android's
// `reboot bootloader` / `reboot recovery` do: the kernel's arch_reset marks
// the target mode in the RTC spare register so LK boots into it.
//
//	rebootto bootloader   # fastboot
//	rebootto recovery
//	rebootto              # plain reboot
package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const restart2 = 0xA1B2C3D4 // LINUX_REBOOT_CMD_RESTART2

func main() {
	arg := ""
	if len(os.Args) > 1 {
		arg = os.Args[1]
	}
	syscall.Sync()
	if arg == "" {
		if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
			fmt.Fprintln(os.Stderr, "reboot:", err)
			os.Exit(1)
		}
		return
	}
	b := append([]byte(arg), 0)
	_, _, e := syscall.Syscall6(syscall.SYS_REBOOT, syscall.LINUX_REBOOT_MAGIC1, syscall.LINUX_REBOOT_MAGIC2, restart2, uintptr(unsafe.Pointer(&b[0])), 0, 0)
	if e != 0 {
		fmt.Fprintln(os.Stderr, "reboot:", e)
		os.Exit(1)
	}
}
