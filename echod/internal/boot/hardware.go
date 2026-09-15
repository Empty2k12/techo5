package boot

import (
	"fmt"
	"os"
	"strings"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

func listenAddr() string { return fmt.Sprintf(":%d", layout.Port) }

func name() string {
	if b, err := os.ReadFile(layout.NamePath); err == nil {
		if recorded := strings.TrimSpace(string(b)); recorded != "" {
			return recorded
		}
	}

	mac, err := layout.FactoryMAC()
	if err != nil {
		return layout.DefaultName
	}
	return layout.NameFromMAC(mac)
}
