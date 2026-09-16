//go:build spot

package home

const (
	// cameraFrameW and H are the size frames are scaled to fit: the sensor's 4:3, which the round
	// panel crops to its circle.
	cameraFrameW = 640
	cameraFrameH = 480

	// localCameraName is the device's own camera on the list, and what "show …" matches.
	localCameraName = "This Spot"
)
