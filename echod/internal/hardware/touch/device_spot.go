//go:build spot

package touch

// The Echo Spot: a Goodix GT5668 reported by MediaTek's mtk-tpd driver, 0 to 480 on both axes, in
// the round panel's own frame.
const (
	deviceName = "mtk-tpd"

	// Width and Height are the frame coordinates are reported in.
	Width  = 480
	Height = 480

	rawFallbackW, rawFallbackH = 481, 481

	// holdGestures: the ring menu opens on a hold.
	holdGestures = true
)

func toFrame(rawW, rawH, rx, ry int) (x, y int) {
	return rx * Width / max(rawW, 1), ry * Height / max(rawH, 1)
}
