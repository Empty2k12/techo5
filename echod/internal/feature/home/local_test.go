package home

import (
	"fmt"
	"testing"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

func TestStationsOf(t *testing.T) {
	m := hass.Media{Children: []hass.Media{
		{Title: "  KXYZ   Public Radio ", ID: "media-source://radio_browser/a", Kind: "audio/mpeg", CanPlay: true},
		{Title: "kxyz public radio", ID: "media-source://radio_browser/b", CanPlay: true}, // the same name again
		{Title: "A folder", ID: "media-source://radio_browser/tag", CanPlay: false},
		{Title: "", ID: "media-source://radio_browser/c", CanPlay: true},
	}}
	got := stationsOf(m, config.RadioLocal)
	if len(got) != 1 || got[0].Name != "KXYZ Public Radio" || got[0].ID != "media-source://radio_browser/a" || got[0].Kind != "audio/mpeg" {
		t.Fatalf("stations = %+v", got)
	}

	var many hass.Media
	for i := 0; i < 250; i++ {
		many.Children = append(many.Children, hass.Media{Title: fmt.Sprintf("Station %d", i), ID: fmt.Sprint(i), CanPlay: true})
	}
	if n := len(stationsOf(many, config.RadioPopular)); n != popularMax {
		t.Errorf("popular list has %d, want %d", n, popularMax)
	}
	if n := len(stationsOf(many, config.RadioLocal)); n != 250 {
		t.Errorf("local list has %d, want all 250", n)
	}
}
