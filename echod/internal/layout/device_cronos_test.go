//go:build !dot && !spot

package layout

import "testing"

func TestBoardName(t *testing.T) {
	// missing is a path no build host has, so only the command line decides.
	const missing = "/nonexistent/amazon-gating"

	for _, c := range []struct {
		cmdline string
		want    string
	}{
		{"bootopt=64S3,32N2,64N2 lcm=1-st7701s_wsvga_dsi_vdo_cronos_st_truly vram=7340032", cronos},
		{"bootopt=64S3,32N2,64N2 lcm=1-st7701s_wsvga_dsi_vdo_checkers_st_truly techo5=linux", checkers},
		{"", cronos},
	} {
		if got := boardName(c.cmdline, missing); got != c.want {
			t.Errorf("boardName(%q) = %q, want %q", c.cmdline, got, c.want)
		}
	}

	// With nothing to read on the command line, the mute driver that is there decides.
	if got := boardName("console=ttyMT0", t.TempDir()); got != checkers {
		t.Errorf("boardName with amazon-gating present = %q, want %q", got, checkers)
	}
}
