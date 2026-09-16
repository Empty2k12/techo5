package update

import (
	"strings"
	"testing"
)

// A release published before Binaries existed describes an arm64 build in its top-level fields, and an
// arm64 device has to go on updating through it — that is the only route off the build it is running.
func TestForReadsAReleaseThatPredatesBinaries(t *testing.T) {
	m := Manifest{
		Version: "0.0.6",
		URL:     "https://example/echod",
		SHA256:  strings.Repeat("a", 64),
		Size:    24 << 20,
	}
	if err := m.Valid(); err != nil {
		t.Fatal(err)
	}

	b, err := m.For("arm64")
	if err != nil {
		t.Fatal(err)
	}
	if b.URL != m.URL || b.SHA256 != m.SHA256 || b.Size != m.Size {
		t.Errorf("offered %+v, want the manifest's own fields", b)
	}

	if _, err := m.For("arm"); err == nil {
		t.Error("offered a 32-bit device a release that only carries arm64")
	}
}

// Every device takes the build for what it is running, and a release carrying both must not hand
// either one the other's.
func TestForTakesTheArchitectureTheDeviceRuns(t *testing.T) {
	m := Manifest{
		Version: "0.0.7",
		URL:     "https://example/echod-arm64",
		SHA256:  strings.Repeat("a", 64),
		Size:    24 << 20,
		Binaries: map[string]Binary{
			"arm64": {URL: "https://example/echod-arm64", SHA256: strings.Repeat("a", 64), Size: 24 << 20},
			"arm":   {URL: "https://example/echod-arm", SHA256: strings.Repeat("b", 64), Size: 22 << 20},
		},
	}
	if err := m.Valid(); err != nil {
		t.Fatal(err)
	}

	for arch, want := range map[string]string{
		"arm64": "https://example/echod-arm64",
		"arm":   "https://example/echod-arm",
	} {
		b, err := m.For(arch)
		if err != nil {
			t.Errorf("%s: %v", arch, err)
			continue
		}
		if b.URL != want {
			t.Errorf("%s: offered %s, want %s", arch, b.URL, want)
		}
	}
}

// A release with only the Show's build serves no Dot, and one with the Dot's serves it. (Off a slot
// system, which is where tests run; on a slot device the rootfs map decides.)
func TestServesOnlyWhatThisDeviceCanInstall(t *testing.T) {
	if slotSystem() {
		t.Skip("running on a slot device")
	}
	restore := arch
	t.Cleanup(func() { arch = restore })
	arch = "arm-dot"

	show := Manifest{Version: "0.0.9", Binaries: map[string]Binary{
		"arm": {URL: "https://example/echod-arm", SHA256: strings.Repeat("b", 64), Size: 22 << 20},
	}}
	if show.Serves() {
		t.Error("a Show-only release was offered to a Dot")
	}
	show.Binaries["arm-dot"] = Binary{URL: "https://example/echod-arm-dot", SHA256: strings.Repeat("c", 64), Size: 22 << 20}
	if !show.Serves() {
		t.Error("a release carrying the Dot's build was not offered to it")
	}
}
