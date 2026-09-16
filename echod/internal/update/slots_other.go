//go:build !linux

package update

import (
	"context"
	"errors"
)

func slotSystem() bool { return false }

func installRootfs(context.Context, Manifest, func(float32)) error {
	return errors.New("update: slot installs only work on the device")
}
