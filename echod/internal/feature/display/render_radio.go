//go:build !dot && !spot

package display

import (
	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
)

// The radio page: what is playing, the stations Home Assistant lists, a Stop row while something
// plays, and the bar that closes the page. Rows fit the 480-row panel: up to seven of 44 from 92.
// radioList is the rows in order: "Stop" first while something plays, then the stations.
func radioList(rd home.Radio) []string {
	var rows []string
	if rd.Playing || rd.Chosen != "" {
		rows = append(rows, "■ Stop")
	}
	return append(rows, rd.Stations...)
}
