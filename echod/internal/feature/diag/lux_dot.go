//go:build dot

package diag

import (
	"log/slog"
	"sync"

	"github.com/HuskerMinion/techo5/echod/internal/hardware/metrics"
)

// luxPath is the sensor file, found once. The Dot's light sensor is an IIO or vendor i2c part the
// metrics reader can open directly.
var luxPath = sync.OnceValue(func() string {
	at := metrics.Reader{}.LuxPath()
	if at == "" {
		slog.Warn("no light sensor found")
	} else {
		slog.Info("light sensor", "at", at)
	}
	return at
})

func lux() metrics.Reading { return metrics.Reader{}.Lux(luxPath()) }
