//go:build dot

package update

// archSuffix marks the Dot's builds, so a release carrying only the Show's arm binary offers this
// device nothing rather than the wrong thing.
const archSuffix = "-dot"
