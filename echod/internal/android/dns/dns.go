// Package dns teaches Go's resolver where this device keeps its nameservers.
//
// Android has no /etc/resolv.conf: bionic reads the nameservers from system properties, and echod is
// built without cgo so it gets Go's own resolver, which only knows about the file. Every lookup
// therefore goes to [::1]:53 and is refused. Nothing noticed for a long time because Home Assistant is
// reached by address and the wake word models come from Home Assistant — the first name anything had to
// resolve was a release manifest.
package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/android/prop"
)

// dhcpcd's hook sets the dhcp ones from the lease. Promoting them to net.dns is the framework's job,
// and there is no framework here.
var props = []string{
	"net.dns1", "net.dns2", "net.dns3", "net.dns4",
	"dhcp.wlan0.dns1", "dhcp.wlan0.dns2", "dhcp.wlan0.dns3", "dhcp.wlan0.dns4",
}

const (
	// timeout bounds one lookup, short enough that a nameserver which is not answering falls through to
	// the next rather than holding up whatever asked.
	timeout = 5 * time.Second

	// fresh is how long the nameservers are trusted before being read again. They come from DHCP and
	// change when the device joins another network, so reading them once would leave a moved device
	// resolving against a nameserver that is no longer there. Reading them every lookup is worse: each
	// one is a getprop, and whatever starts resolving in a loop later would be spawning processes to do
	// it. A minute is short against how often a device changes network and long against anything that
	// resolves in earnest.
	fresh = time.Minute
)

// Use makes every lookup in this process go to the nameservers the platform reports, by replacing the
// resolver the standard library shares. Nothing else has to be told: http.DefaultClient included.
func Use() {
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: dial}
	slog.Info("resolver configured", "nameservers", nameservers())
}

// dial ignores the address Go derived from the resolv.conf it could not find and uses what the platform
// says, trying each in turn.
func dial(ctx context.Context, network, _ string) (net.Conn, error) {
	servers := nameservers()
	if len(servers) == 0 {
		return nil, fmt.Errorf("dns: the device reports no nameservers")
	}

	var err error
	for _, server := range servers {
		d := net.Dialer{Timeout: timeout}

		var conn net.Conn
		if conn, err = d.DialContext(ctx, network, net.JoinHostPort(server, "53")); err == nil {
			return conn, nil
		}
	}
	return nil, err
}

var (
	mu     sync.Mutex
	cached []string
	read   time.Time
)

func nameservers() []string {
	mu.Lock()
	defer mu.Unlock()

	if cached != nil && time.Since(read) < fresh {
		return cached
	}

	var out []string
	for _, name := range props {
		v, err := prop.Get(name)
		if err != nil || v == "" || slices.Contains(out, v) {
			continue
		}
		out = append(out, v)
	}

	// Newer Android keeps the nameservers inside netd rather than in properties, so a device on
	// LineageOS reports none this way. The default gateway is the next best guess: on a home network
	// it is the router, and the router resolves.
	if len(out) == 0 {
		if gw := gateway(); gw != "" {
			out = append(out, gw)
		}
	}

	cached, read = out, time.Now()
	return out
}

// gateway reads the default route from /proc/net/route, where the gateway is a little-endian hex
// address in the third column.
func gateway() string {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || f[1] != "00000000" {
			continue
		}
		var a, b, c, d byte
		if _, err := fmt.Sscanf(f[2], "%02x%02x%02x%02x", &d, &c, &b, &a); err != nil {
			continue
		}
		return net.IPv4(a, b, c, d).String()
	}
	return ""
}
