//go:build spot

package update

// archSuffix marks the Spot's builds, so a release carrying another device's binary offers this one
// nothing rather than the wrong thing.
const archSuffix = "-spot"
