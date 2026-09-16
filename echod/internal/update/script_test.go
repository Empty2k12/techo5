package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// The two hooks run at different moments and must not be interchangeable: only the one init runs on the
// way up may put a binary back, because the other fires while a healthy update is still on trial.
func TestOnlyTheEarlyHookRollsBack(t *testing.T) {
	if len(layout.AnimationScripts) == 0 {
		t.Skip("this device has no vendor boot hooks to take over")
	}
	early := Script(layout.StartAnimation)
	late := Script(layout.StopAnimation)

	if !strings.Contains(early, layout.PrevBinary) {
		t.Errorf("the boot hook does not mention %s:\n%s", layout.PrevBinary, early)
	}
	if strings.Contains(late, layout.PrevBinary) {
		t.Errorf("the hook that runs after the boot would roll back:\n%s", late)
	}
	if early == late {
		t.Error("both hooks have the same contents")
	}
}

// Whatever else it does, it has to end in a way init is happy with and never run ledctrl, which is the
// original reason for replacing it.
func TestBothHooksExitCleanlyAndNeverCallLedctrl(t *testing.T) {
	for _, path := range layout.AnimationScripts {
		script := Script(path)

		if !strings.HasPrefix(script, "#!/system/bin/sh\n") {
			t.Errorf("%s has no shebang:\n%s", path, script)
		}
		if !strings.HasSuffix(script, "exit 0\n") {
			t.Errorf("%s does not end by exiting cleanly:\n%s", path, script)
		}
		// The comment explains what was replaced, so what must be absent is the executable itself.
		if strings.Contains(script, "/system/bin/ledctrl") {
			t.Errorf("%s still calls ledctrl", path)
		}
	}
}

// The rollback has to leave the device in the state everything else expects: the binary back, /system
// read-only again, and the failure recorded where echod will find it.
func TestRollbackRestoresAndReports(t *testing.T) {
	if len(layout.AnimationScripts) == 0 {
		t.Skip("this device has no vendor boot hooks; rollback is done in-process by update.Start")
	}
	script := Script(layout.StartAnimation)

	for _, want := range []string{
		"mount -o remount,rw /system",
		"mv -f " + layout.PrevBinary + " " + layout.Binary,
		"mount -o remount,ro /system",
		"setprop " + layout.RolledBackProp,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the rollback is missing %q:\n%s", want, script)
		}
	}
}

// Ensure only writes what differs, because an ordinary start should not be remounting /system at all.
func TestEnsureLeavesCurrentHooksAlone(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "start.sh"), filepath.Join(dir, "stop.sh")}

	restore := layout.AnimationScripts
	t.Cleanup(func() { layout.AnimationScripts = restore; writable = remount; onAndroid = layout.OnAndroid })
	layout.AnimationScripts = paths
	onAndroid = func() bool { return true }

	var remounts int
	writable = func(bool) error { remounts++; return nil }

	// Written as this build wants them, so there is nothing to do.
	for _, p := range paths {
		if err := os.WriteFile(p, []byte(Script(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	Ensure()
	if remounts != 0 {
		t.Errorf("remounted %d times with both hooks already current", remounts)
	}

	// One of them behind, so it is rewritten.
	if err := os.WriteFile(paths[0], []byte("#!/system/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	Ensure()
	if remounts == 0 {
		t.Error("a stale hook was left alone")
	}
	if got, err := os.ReadFile(paths[0]); err != nil || string(got) != Script(paths[0]) {
		t.Errorf("the stale hook was not rewritten: %v", err)
	}
}

// Off Android the hooks are not Fire OS's to keep: nothing is written and nothing remounted, however
// stale they look. On the Dot's Linux image the remount was of the root filesystem itself.
func TestEnsureStandsDownOffAndroid(t *testing.T) {
	dir := t.TempDir()
	restore := layout.AnimationScripts
	t.Cleanup(func() { layout.AnimationScripts = restore; writable = remount; onAndroid = layout.OnAndroid })
	layout.AnimationScripts = []string{filepath.Join(dir, "start.sh")}
	onAndroid = func() bool { return false }

	var remounts int
	writable = func(bool) error { remounts++; return nil }
	Ensure()
	if remounts != 0 {
		t.Errorf("remounted %d times off Android", remounts)
	}
	if _, err := os.Stat(layout.AnimationScripts[0]); err == nil {
		t.Error("a hook was written off Android")
	}
}
