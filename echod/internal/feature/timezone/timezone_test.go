package timezone

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValid(t *testing.T) {
	for name, want := range map[string]bool{
		"America/Chicago": true, "Europe/Berlin": true, "UTC": true, "Etc/GMT+5": true, "America/Port-au-Prince": true,
		"": false, "../../etc/passwd": false, "/etc/localtime": false, "America/Chi cago": false, "Zone;rm": false,
	} {
		if got := valid(name); got != want {
			t.Errorf("valid(%q) = %v, want %v", name, got, want)
		}
	}
}

// A zone from Home Assistant lands on userdata as a name and a link into zoneinfo, and takes effect in
// the running process; one this image does not have changes nothing.
func TestApply(t *testing.T) {
	if _, err := time.LoadLocation("Europe/Berlin"); err != nil {
		t.Skip("no zoneinfo on this machine:", err)
	}
	system := "/usr/share/zoneinfo"
	if _, err := os.Stat(filepath.Join(system, "Europe/Berlin")); err != nil {
		t.Skip("no zoneinfo directory:", err)
	}
	dir := t.TempDir()
	nameFile, linkFile, zoneinfo = filepath.Join(dir, "timezone"), filepath.Join(dir, "localtime"), system
	saved := time.Local
	defer func() { time.Local = saved }()

	z := &Zone{}
	if err := z.apply("Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	if got := z.Current(); got != "Europe/Berlin" {
		t.Errorf("saved zone = %q", got)
	}
	if target, err := os.Readlink(linkFile); err != nil || target != filepath.Join(system, "Europe/Berlin") {
		t.Errorf("link = %q, %v", target, err)
	}
	if time.Local.String() != "Europe/Berlin" {
		t.Errorf("time.Local = %s", time.Local)
	}

	if err := z.apply("Nowhere/Atlantis!"); err == nil {
		t.Error("an unknown zone was applied")
	}
	if got := z.Current(); got != "Europe/Berlin" {
		t.Errorf("an unknown zone replaced the saved one: %q", got)
	}
	if err := z.apply(""); err != nil || z.Current() != "Europe/Berlin" {
		t.Errorf("an empty zone changed something: %v %q", err, z.Current())
	}
}

// The rule Home Assistant sends becomes a zoneinfo file with the right offsets either side of the
// daylight saving change, in Go and for anything else that reads /etc/localtime.
func TestApplyPOSIXRule(t *testing.T) {
	dir := t.TempDir()
	nameFile, linkFile, zoneinfo = filepath.Join(dir, "timezone"), filepath.Join(dir, "localtime"), filepath.Join(dir, "no-zoneinfo")
	saved := time.Local
	defer func() { time.Local = saved }()

	z := &Zone{}
	rule := "MST7MDT,M3.2.0,M11.1.0"
	if err := z.apply(rule); err != nil {
		t.Fatal(err)
	}
	if got := z.Current(); got != rule {
		t.Errorf("saved zone = %q", got)
	}
	data, err := os.ReadFile(linkFile)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocationFromTZData("file", data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		when   time.Time
		name   string
		offset int
	}{
		{time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC), "MST", -7 * 3600},
		{time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "MDT", -6 * 3600},
		{time.Date(2026, 3, 8, 8, 59, 0, 0, time.UTC), "MST", -7 * 3600}, // 01:59 MST, a minute before the change
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC), "MDT", -6 * 3600},  // 03:00 MDT
		{time.Date(2040, 11, 5, 12, 0, 0, 0, time.UTC), "MST", -7 * 3600},
	} {
		name, offset := tc.when.In(loc).Zone()
		if name != tc.name || offset != tc.offset {
			t.Errorf("%s: %s %d, want %s %d", tc.when, name, offset, tc.name, tc.offset)
		}
	}
	if name, _ := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC).In(time.Local).Zone(); name != "MDT" {
		t.Errorf("time.Local in July is %s", name)
	}

	for _, bad := range []string{"garbage", "MST7MDT;rm -rf /", "M3.2.0"} {
		if err := z.apply(bad); err == nil {
			t.Errorf("applied %q", bad)
		}
	}
	if got := z.Current(); got != rule {
		t.Errorf("a bad rule replaced the saved one: %q", got)
	}
	if err := z.apply("UTC0"); err != nil {
		t.Errorf("UTC0: %v", err)
	}
}
