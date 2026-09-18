//go:build spot

package home

const (
	// cameraFrameW and H are the size frames are scaled to fit: the sensor's 4:3, which the round
	// panel crops to its circle.
	cameraFrameW = 640
	cameraFrameH = 480

	// localCameraName is the device's own camera on the list, and what "show …" matches.
	localCameraName = "This Spot"

	// slideshowW and H are the panel's own size, 480×480 — a slideshow photo is cropped to fill this
	// square directly, not artW/artH's 960×480 (that would crop twice: once to a landscape rectangle,
	// then again to the panel's actual square when drawn).
	slideshowW = 480
	slideshowH = 480
)
