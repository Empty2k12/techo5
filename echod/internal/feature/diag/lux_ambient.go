//go:build !dot

package diag

import (
	"github.com/HuskerMinion/techo5/echod/internal/hardware/ambient"
	"github.com/HuskerMinion/techo5/echod/internal/hardware/metrics"
)

// lux comes from the hwmsensor stream hardware/ambient reads, not from a file: this board has no
// IIO illuminance device and no vendor als_lux, so metrics.LuxPath finds nothing.
func lux() metrics.Reading {
	v, _, ok := ambient.Get().Current()
	return metrics.Reading{Value: v, Known: ok}
}
