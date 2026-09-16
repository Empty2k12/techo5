//go:build spot

package all

// notOnThisDevice is what the Echo Spot build does not put up yet: its camera and the web pages.
var notOnThisDevice = []string{"camera_web_access", "screen_web_access"}
