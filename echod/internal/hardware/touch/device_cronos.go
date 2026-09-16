//go:build !dot && !spot

package touch

// The Echo Show 5: a Goodix GT9xx in the panel's portrait frame, 480 wide and 960 tall, while the
// device sits landscape. Landscape x runs along the panel's y, landscape y runs back along its x.
const (
	deviceName = "goodix-ts"

	// Width and Height are the frame coordinates are reported in.
	Width  = 960
	Height = 480

	rawFallbackW, rawFallbackH = 480, 960

	// holdGestures: the Show's screen has no use for holds; its taps and swipes stay as they were.
	holdGestures = false
)

func toFrame(rawW, rawH, rx, ry int) (x, y int) {
	x = ry * Width / max(rawH, 1)
	y = (rawW - 1 - rx) * Height / max(rawW, 1)
	return x, y
}
