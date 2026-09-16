package security

import (
	"strings"
	"testing"
)

func TestParseKeys(t *testing.T) {
	ed := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample someone@desk"
	rsa := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQExample"

	keys, err := parseKeys("\r\n  " + ed + "  \r\n\n" + rsa + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != ed || keys[1] != rsa {
		t.Fatalf("keys = %q", keys)
	}
	if got := keyLabel(keys[0]); got != "someone@desk" {
		t.Errorf("label = %q", got)
	}
	if got := keyLabel(keys[1]); got != "ssh-rsa" {
		t.Errorf("label without a comment = %q", got)
	}

	if keys, err := parseKeys("  \n"); err != nil || len(keys) != 0 {
		t.Errorf("empty = %q, %v", keys, err)
	}

	for _, bad := range []string{
		`command="rm -rf /" ` + ed, // options in front of a key run things
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"ssh-ed25519",
		"hello world",
	} {
		if _, err := parseKeys(ed + "\n" + bad); err == nil {
			t.Errorf("accepted %q", bad)
		} else if strings.Contains(err.Error(), "AAAA") && strings.Contains(bad, "PRIVATE") {
			t.Errorf("error echoes key material: %v", err)
		}
	}
}
