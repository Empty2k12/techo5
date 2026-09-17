package phone

import (
	"context"
	"log/slog"
	"time"
)

// slowDNS is how long a name lookup takes before it is worth a warning. sipgo warns past 50 ms, which a
// home network's resolver takes routinely (60 to 100 ms seen on every device), so the log filled with
// warnings about nothing.
const slowDNS = time.Second

// sipLogger is the logger sipgo gets: the daemon's own, with its slow-lookup warning kept only for
// lookups that are actually slow.
func sipLogger() *slog.Logger { return slog.New(quietDNS{slog.Default().Handler()}) }

// quietDNS passes everything through except sipgo's "DNS resolution is slow" under slowDNS, which it
// drops.
type quietDNS struct{ slog.Handler }

func (h quietDNS) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "DNS resolution is slow" {
		slow := true
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "dur" {
				if d, ok := a.Value.Any().(time.Duration); ok && d < slowDNS {
					slow = false
				}
				return false
			}
			return true
		})
		if !slow {
			return nil
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h quietDNS) WithAttrs(attrs []slog.Attr) slog.Handler {
	return quietDNS{h.Handler.WithAttrs(attrs)}
}

func (h quietDNS) WithGroup(name string) slog.Handler { return quietDNS{h.Handler.WithGroup(name)} }
