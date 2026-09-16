package main

// h4Framer cuts a byte stream from the controller into whole H4 packets.
//
// The Echo Show's vendor module hands out one packet per read, but the Echo Dot's built-in driver
// (conn_soc stp_chrdev_bt.c) copies whatever its ring buffer holds: one read can carry several
// packets or half of one. /dev/vhci takes exactly one packet per write, so the stream is framed here
// from the length fields in each packet's header. For a driver that already returns whole packets
// this changes nothing.
type h4Framer struct {
	buf []byte
	// dropped counts bytes skipped because they did not start a known packet type.
	dropped int
}

const (
	h4ACL   = 0x02
	h4SCO   = 0x03
	h4Event = 0x04
	h4ISO   = 0x05
)

// feed appends what a read returned and returns every packet now complete, in order. The returned
// slices stay valid until the next feed.
func (f *h4Framer) feed(b []byte) [][]byte {
	f.buf = append(f.buf, b...)
	var out [][]byte
	for len(f.buf) > 0 {
		n := packetLen(f.buf)
		if n < 0 {
			// Not a packet type the controller sends: out of step, so skip a byte and look again.
			f.buf = f.buf[1:]
			f.dropped++
			continue
		}
		if n == 0 || len(f.buf) < n {
			break // header or body still to come
		}
		out = append(out, f.buf[:n:n])
		f.buf = f.buf[n:]
	}
	// Keep the partial packet at the front of a fresh backing array, so the ones handed out are not
	// overwritten by the next append.
	f.buf = append([]byte(nil), f.buf...)
	return out
}

// capFeaturePages lowers the maximum page number in a successful Read Local Extended Features reply
// to max. The Echo Dot's MediaTek controller reports two pages beyond the first and then answers a
// read of page 2 with "Parameter Out Of Mandatory Range"; the 3.18 kernel treats that as a failed
// init and never brings hci0 up, so BlueZ never sees it. Newer kernels carry a quirk for this.
// Reports whether the packet was changed.
func capFeaturePages(pkt []byte, max byte) bool {
	// 04 0e plen ncmd opcode(04 10) status page max_page features[8]
	if len(pkt) < 9 || pkt[0] != h4Event || pkt[1] != 0x0e || pkt[4] != 0x04 || pkt[5] != 0x10 {
		return false
	}
	if pkt[6] != 0 || pkt[8] <= max {
		return false
	}
	pkt[8] = max
	return true
}

// packetLen is the whole length of the packet at the start of b: -1 for an unknown type, 0 when the
// header is not all there yet.
func packetLen(b []byte) int {
	switch b[0] {
	case h4Event: // type, event code, length (1)
		if len(b) < 3 {
			return 0
		}
		return 3 + int(b[2])
	case h4ACL: // type, handle (2), length (2)
		if len(b) < 5 {
			return 0
		}
		return 5 + (int(b[3]) | int(b[4])<<8)
	case h4SCO: // type, handle (2), length (1)
		if len(b) < 4 {
			return 0
		}
		return 4 + int(b[3])
	case h4ISO: // type, handle (2), length (2, top two bits are flags)
		if len(b) < 5 {
			return 0
		}
		return 5 + (int(b[3]) | int(b[4]&0x3f)<<8)
	}
	return -1
}
