package phone

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestQuietDNSDropsOnlyOrdinaryLookups(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(quietDNS{slog.NewTextHandler(&buf, nil)}).With("caller", "TransportLayer")

	log.Warn("DNS resolution is slow", "dur", 98*time.Millisecond)
	if buf.Len() != 0 {
		t.Fatalf("a 98 ms lookup was logged: %s", buf.String())
	}
	log.Warn("DNS resolution is slow", "dur", 3*time.Second)
	if !strings.Contains(buf.String(), "dur=3s") {
		t.Fatalf("a 3 s lookup was not logged: %q", buf.String())
	}
	buf.Reset()
	log.Warn("Doing SRV lookup failed.", "host", "example")
	if !strings.Contains(buf.String(), "SRV lookup failed") {
		t.Fatalf("another warning was dropped: %q", buf.String())
	}
}
