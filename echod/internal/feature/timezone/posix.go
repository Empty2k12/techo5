package timezone

import (
	"bytes"
	"encoding/binary"
	"errors"
	"regexp"
	"strings"
	"time"
)

// Home Assistant answers GetTimeRequest with a POSIX TZ rule, the form ESPHome devices take — for
// example "MST7MDT,M3.2.0,M11.1.0" — rather than a zone name. A zoneinfo file can carry such a rule
// as its footer (TZif version 2 and later), where Go and musl both apply it to every time after the
// file's last transition. tzif builds that file: one transition, far in the past, and the rule.

// posixRule is the character set of a POSIX TZ string, bounded so nothing odd is written anywhere.
var posixRule = regexp.MustCompile(`^[A-Za-z<>+\-0-9,.:/]{3,64}$`)

// tzif is a zoneinfo file whose only content is rule.
func tzif(rule string) []byte {
	var b bytes.Buffer
	be := func(v any) { _ = binary.Write(&b, binary.BigEndian, v) }
	header := func(timecnt uint32) {
		b.WriteString("TZif2")
		b.Write(make([]byte, 15))
		// isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt
		for _, n := range []uint32{0, 0, 0, timecnt, 1, 4} {
			be(n)
		}
	}
	types := func() {
		be(int32(0)) // offset of the placeholder type, used only before the transition
		b.WriteByte(0)
		b.WriteByte(0)
		b.WriteString("UTC\x00")
	}
	// Version 1 block: no transitions, 32-bit times; readers of version 2 skip it.
	header(0)
	types()
	// Version 2 block: one transition in 1901, so every later time is past the last one and the rule
	// decides it. With no transitions at all, Go would use the placeholder type instead of the rule.
	header(1)
	be(int64(-1 << 31))
	b.WriteByte(0)
	types()
	b.WriteString("\n" + rule + "\n")
	return b.Bytes()
}

// loadRule is rule as a location, and an error when rule is not one Go can apply: Go ignores a footer
// it cannot parse and would silently leave the placeholder in force.
func loadRule(rule string) (*time.Location, error) {
	if !posixRule.MatchString(rule) {
		return nil, errors.New("not a POSIX TZ rule")
	}
	loc, err := time.LoadLocationFromTZData(rule, tzif(rule))
	if err != nil {
		return nil, err
	}
	// The abbreviations in force in January and July must come from the rule, not the placeholder.
	for _, month := range []time.Month{time.January, time.July} {
		name, _ := time.Date(time.Now().Year(), month, 15, 12, 0, 0, 0, loc).Zone()
		if name == "UTC" && !strings.HasPrefix(rule, "UTC") {
			return nil, errors.New("rule not understood")
		}
	}
	return loc, nil
}
