//go:build !spot

package home

const (
	// cameraFrameW and H are the size frames are scaled to fit: the Show's panel.
	cameraFrameW = 960
	cameraFrameH = 480

	// localCameraName is the device's own camera on the list, and what "show …" matches.
	localCameraName = "This Show"

	// slideshowW and H are the panel's own size — the Show's screen, same as cameraFrameW/H. (This
	// file's !spot build tag also covers the Dot, which never uses these: hasScreen is false there.)
	slideshowW = 960
	slideshowH = 480
)
