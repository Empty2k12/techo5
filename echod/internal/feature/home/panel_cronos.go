//go:build !spot

package home

const (
	// cameraFrameW and H are the size frames are scaled to fit: the Show's panel.
	cameraFrameW = 960
	cameraFrameH = 480

	// localCameraName is the device's own camera on the list, and what "show …" matches.
	localCameraName = "This Show"
)
