package phone

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// Contacts are the people and places a screen offers to call, set from Home Assistant (the
// phone_contacts action) so a device with a screen can place a call without a voice command. Phone
// numbers are personal, so they are kept like the login: their own owner-only file, not state.json.
var contactsPath = filepath.Join(layout.StateDir, "phone-contacts.json")

// Contact is one entry: a name to show and what to dial.
type Contact struct {
	Name   string `json:"name"`
	Number string `json:"number"`
}

// maxContacts is as many as a screen can list without scrolling.
const maxContacts = 12

// Contacts is the list, in the order it was given.
func (p *Phone) Contacts() []Contact {
	b, err := os.ReadFile(contactsPath)
	if err != nil {
		return nil
	}
	var out []Contact
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// parseContacts reads "Name=number, Name=number"; an empty string is an empty list.
func parseContacts(s string) ([]Contact, error) {
	var out []Contact
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, number, ok := strings.Cut(part, "=")
		name, number = strings.TrimSpace(name), dialable(number)
		if !ok || name == "" || number == "" {
			return nil, fmt.Errorf("phone: %q is not name=number", part)
		}
		out = append(out, Contact{Name: name, Number: number})
	}
	if len(out) > maxContacts {
		return nil, fmt.Errorf("phone: %d contacts, at most %d fit on a screen", len(out), maxContacts)
	}
	return out, nil
}

func saveContacts(list []Contact) error {
	if len(list) == 0 {
		err := os.Remove(contactsPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp := contactsPath + ".new"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, contactsPath)
}
