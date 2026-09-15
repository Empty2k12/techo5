//go:build !dot

package config

// DefaultMixing on the Echo Show 5: both microphones averaged, which is what the vendor's capture
// driver did before the daemon took the channels apart. "Center mic" would be one of the two.
const DefaultMixing = MixAll
