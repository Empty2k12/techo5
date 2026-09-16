package security

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// Where SSH keeps what it needs. The image links /root/.ssh to KeysDir, so the keys live on
// userdata, survive slot changes, and are never part of an image; the host keys are on userdata
// for the same reason (/etc/dropbear links there).
const (
	KeysDir  = layout.StateDir + "/ssh"
	keysFile = KeysDir + "/authorized_keys"
	hostKeys = "/data/techo5-linux/dropbear"
	pidFile  = "/run/dropbear.pid"

	// slotMarker is left by the initramfs on a normal slot boot. The rescue environment runs the
	// daemon too, and has its own SSH server that this must not take away.
	slotMarker = "/run/techo5/slot"
)

// sshAvailable reports whether the daemon manages SSH here: a slot boot of the Linux image.
func sshAvailable() bool {
	if layout.OnAndroid() {
		return false
	}
	if _, err := os.Stat(slotMarker); err != nil {
		return false
	}
	_, err := exec.LookPath("dropbear")
	return err == nil
}

// sshPID is the listening server's process, or 0. Sessions are children with the same name; the
// pid file names only the listener.
func sshPID() int {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0
	}
	comm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil || strings.TrimSpace(string(comm)) != "dropbear" {
		return 0
	}
	return pid
}

func sshRunning() bool { return sshPID() != 0 }

// startSSH starts dropbear, which puts itself in the background. Keys only: -s turns password
// logins off for every account.
func startSSH() error {
	if err := os.MkdirAll(hostKeys, 0o700); err != nil {
		return err
	}
	out, err := exec.Command("dropbear", "-R", "-s", "-p", "22", "-P", pidFile).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(err.Error() + ": " + string(out)))
	}
	return nil
}

// stopSSH stops the listener. Sessions already open are their own processes and carry on, so
// switching SSH off from inside an SSH session does not cut it.
func stopSSH() error {
	pid := sshPID()
	if pid == 0 {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func readKeys() []string {
	b, err := os.ReadFile(keysFile)
	if err != nil {
		return nil
	}
	keys, _ := parseKeys(string(b))
	return keys
}

// writeKeys replaces the file through a temporary one; dropbear refuses keys whose file or
// directory others can write, so both are the owner's alone.
func writeKeys(keys []string) error {
	if err := os.MkdirAll(KeysDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(KeysDir, 0o700); err != nil {
		return err
	}
	if len(keys) == 0 {
		err := os.Remove(keysFile)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	tmp := filepath.Join(KeysDir, ".authorized_keys.new")
	if err := os.WriteFile(tmp, []byte(strings.Join(keys, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, keysFile)
}
