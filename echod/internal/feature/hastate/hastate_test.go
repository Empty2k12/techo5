package hastate

import (
	"context"
	"testing"

	"github.com/ygelfand/go-esphome-device/api"
)

func TestUpdateFirst(t *testing.T) {
	tr := &Tracker{values: map[Key]string{}}
	var got []Update
	tr.Changed.Listen(func(u Update) { got = append(got, u) })
	send := func(v string) {
		if err := tr.Handle(context.Background(), nil, &api.HomeAssistantStateResponse{EntityId: "input_text.last_station", State: v}); err != nil {
			t.Fatal(err)
		}
	}
	send("KXYZ")
	send("KXYZ") // the same again says nothing
	send("WKRP")
	if len(got) != 2 || !got[0].First || got[1].First || got[1].Value != "WKRP" {
		t.Fatalf("updates = %+v, want the first marked First and the change not", got)
	}
}
