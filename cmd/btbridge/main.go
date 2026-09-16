//go:build linux

// btbridge makes the MediaTek Bluetooth driver's raw H4 channel (/dev/stpbt,
// from the vendor mt76x8_bt module) look like a Linux HCI device by copying
// packets both ways through the kernel's virtual HCI driver (/dev/vhci,
// CONFIG_BT_HCIVHCI). BlueZ then sees hci0 like on any other board.
//
//	btbridge [-stp /dev/stpbt] [-vhci /dev/vhci]
//
// Each read on either side returns one whole H4 packet (type byte first);
// each write must be one whole packet. The vendor driver's read returns 0
// bytes when its queue is empty instead of blocking, so that side is polled
// with select(2) and a short timeout. On any error the bridge exits after a
// pause so init's respawn does not spin.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

const vendorPkt = 0xff // HCI_VENDOR_PKT: vhci control packets, never forwarded

func main() {
	stpPath := flag.String("stp", "/dev/stpbt", "vendor driver's H4 character device")
	vhciPath := flag.String("vhci", "/dev/vhci", "kernel virtual HCI device")
	bdaddr := flag.String("bdaddr", "idme", "public address to give the controller first: 12 hex digits, "+
		"\"idme\" for the factory one from /proc/idme/bt_mac_addr, \"\" to leave the firmware's")
	flag.Parse()

	stp, err := syscall.Open(*stpPath, syscall.O_RDWR, 0)
	if err != nil {
		die("open %s: %v", *stpPath, err)
	}
	if *bdaddr == "idme" {
		b, err := os.ReadFile("/proc/idme/bt_mac_addr")
		if err != nil {
			fmt.Fprintf(os.Stderr, "btbridge: no factory address: %v\n", err)
		}
		*bdaddr = strings.TrimRight(strings.TrimSpace(string(b)), "\x00")
	}
	if *bdaddr != "" {
		if err := setBdaddr(stp, *bdaddr); err != nil {
			fmt.Fprintf(os.Stderr, "btbridge: set address %s: %v (keeping the controller's own)\n", *bdaddr, err)
		} else {
			fmt.Fprintf(os.Stderr, "btbridge: controller address set to %s\n", *bdaddr)
		}
	}
	vhci, err := os.OpenFile(*vhciPath, os.O_RDWR, 0)
	if err != nil {
		die("open %s: %v", *vhciPath, err)
	}
	// Register a primary (BR/EDR + LE) controller at once instead of waiting
	// for the driver's one-second timeout.
	if _, err := vhci.Write([]byte{vendorPkt, 0x00}); err != nil {
		die("vhci: create device: %v", err)
	}
	fmt.Fprintln(os.Stderr, "btbridge: hci device registered")

	errc := make(chan error, 2)
	go func() { errc <- vhciToStp(vhci, stp) }()
	go func() { errc <- stpToVhci(stp, vhci) }()
	die("%v", <-errc)
}

// vhciToStp forwards packets the kernel stack sends (vhci reads block).
func vhciToStp(vhci *os.File, stp int) error {
	buf := make([]byte, 65536)
	for {
		n, err := vhci.Read(buf)
		if err != nil {
			return fmt.Errorf("vhci: read: %w", err)
		}
		if n == 0 || buf[0] == vendorPkt {
			continue // vhci's "device created" notice and the like
		}
		if err := writeAll(stp, buf[:n]); err != nil {
			return fmt.Errorf("stp: write %d bytes: %w", n, err)
		}
	}
}

// stpToVhci forwards packets from the controller; waits with select when the
// driver has nothing queued. What a read returns is framed into whole packets
// first (h4.go): the Echo Dot's driver does not keep packet boundaries.
func stpToVhci(stp int, vhci *os.File) error {
	buf := make([]byte, 65536)
	var framer h4Framer
	dropped := 0
	for {
		n, err := syscall.Read(stp, buf)
		if err == syscall.EINTR || err == syscall.EAGAIN {
			continue
		}
		if err != nil {
			return fmt.Errorf("stp: read: %w", err)
		}
		if n == 0 {
			wait(stp)
			continue
		}
		for _, pkt := range framer.feed(buf[:n]) {
			fixSupportedCommands(pkt)
			if _, err := vhci.Write(pkt); err != nil {
				return fmt.Errorf("vhci: write %d bytes: %w", len(pkt), err)
			}
		}
		if framer.dropped != dropped {
			fmt.Fprintf(os.Stderr, "btbridge: skipped %d bytes that began no packet\n", framer.dropped-dropped)
			dropped = framer.dropped
		}
	}
}

// wait blocks until the fd is readable or 50 ms pass, whichever comes first,
// so a driver without a working poll still gets serviced promptly.
func wait(fd int) {
	var set syscall.FdSet
	set.Bits[fd/32] |= 1 << (uint(fd) % 32)
	tv := syscall.Timeval{Usec: 50000}
	_, _ = syscall.Select(fd+1, &set, nil, nil, &tv)
}

// fixSupportedCommands fills in the LE part of the controller's Read Local
// Supported Commands reply. The MT7668 firmware leaves octets 25-28 empty
// although it runs every command they stand for; the kernel builds the LE
// event mask from that table, so without them it never asks for advertising
// reports or LE connection events and every LE scan comes back empty. Only a
// reply with all four octets zero is touched.
func fixSupportedCommands(p []byte) {
	// 04 0E plen ncmd 02 10 status, then 64 octets of commands
	const first = 7
	if len(p) < first+64 || p[0] != 0x04 || p[1] != 0x0e || p[4] != 0x02 || p[5] != 0x10 || p[6] != 0 {
		return
	}
	le := p[first+25 : first+29]
	if le[0]|le[1]|le[2]|le[3] != 0 {
		return
	}
	// The Bluetooth 4.0 LE commands: octet 25 has a reserved bit 3, octet 28 ends at bit 6.
	copy(le, []byte{0xf7, 0xff, 0xff, 0x7f})
}

// setBdaddr sends MediaTek's vendor Set_BD_ADDR command (opcode 0xFC1A, six
// address bytes little-endian) straight to the controller before the kernel
// stack sees it, so hci0 comes up with the factory address rather than the
// firmware's default. Waits up to two seconds for the Command Complete.
func setBdaddr(stp int, hexaddr string) error {
	if len(hexaddr) != 12 {
		return fmt.Errorf("want 12 hex digits")
	}
	cmd := []byte{0x01, 0x1a, 0xfc, 0x06}
	for i := 5; i >= 0; i-- { // little-endian on the wire
		var v byte
		if _, err := fmt.Sscanf(hexaddr[2*i:2*i+2], "%02x", &v); err != nil {
			return fmt.Errorf("bad address")
		}
		cmd = append(cmd, v)
	}
	if err := writeAll(stp, cmd); err != nil {
		return err
	}
	buf := make([]byte, 512)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, err := syscall.Read(stp, buf)
		if err != nil && err != syscall.EINTR && err != syscall.EAGAIN {
			return err
		}
		if n == 0 {
			wait(stp)
			continue
		}
		// Command Complete for 0xFC1A: 04 0E len ncmd 1A FC status
		if n >= 7 && buf[0] == 0x04 && buf[1] == 0x0e && buf[4] == 0x1a && buf[5] == 0xfc {
			if buf[6] != 0 {
				return fmt.Errorf("controller status 0x%02x", buf[6])
			}
			return nil
		}
	}
	return fmt.Errorf("no reply")
}

func writeAll(fd int, b []byte) error {
	full := 0
	for len(b) > 0 {
		n, err := syscall.Write(fd, b)
		if err == syscall.EINTR {
			continue
		}
		// The Dot's driver answers ENOSPC while its transmit queue is full: wait
		// for it to drain rather than tearing the bridge down (up to a second).
		if err == syscall.ENOSPC && full < 100 {
			full++
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "btbridge: "+format+"\n", args...)
	time.Sleep(3 * time.Second)
	os.Exit(1)
}
