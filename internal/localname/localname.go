// Package localname owns a selected local-only hostname, never a public route.
package localname

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdamNi-7080/AIOS/internal/lifecycle"
)

var pattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func Validate(name string) error {
	if !pattern.MatchString(name) || name == "localhost" || name == "local" {
		return fmt.Errorf("namespace must be 1–63 lowercase DNS-label characters")
	}
	return nil
}

type nativeRecord interface {
	Close()
	Health() error
}
type Lease struct {
	Name, Host string
	Native     bool
	lock       *os.File
	record     nativeRecord
	once       sync.Once
	closed     atomic.Bool
}

func Acquire(name string) (*Lease, error) {
	if e := Validate(name); e != nil {
		return nil, e
	}
	temp, e := filepath.EvalSymlinks(os.TempDir())
	if e != nil {
		return nil, e
	}
	root := filepath.Join(temp, fmt.Sprintf("aios-namespaces-%d", os.Getuid()))
	if e = lifecycle.PrepareDir(root); e != nil {
		return nil, e
	}
	lock, e := lifecycle.Lock(filepath.Join(root, name))
	if e != nil {
		return nil, fmt.Errorf("namespace is already reserved or unavailable")
	}
	l := &Lease{Name: name, Host: name + ".localhost", Native: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64", lock: lock}
	if l.Native {
		l.Host = name + ".local"
		l.record, e = registerNative(l.Host)
		if e != nil {
			l.Close()
			return nil, fmt.Errorf("native namespace collision or registration unavailable: %w", e)
		}
		if e = verifyLoopback(l.Host); e != nil {
			l.Close()
			return nil, e
		}
	}
	return l, nil
}

// A unique A record alone cannot rule out an existing non-loopback AAAA.
// Require the actual system resolver's complete answer before exposing links.
func verifyLoopback(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil || len(addresses) == 0 {
		return fmt.Errorf("selected native name did not resolve locally: %w", e)
	}
	for _, address := range addresses {
		if !address.IP.IsLoopback() {
			return fmt.Errorf("selected native name resolves outside loopback")
		}
	}
	return nil
}
func (l *Lease) Health() error {
	if l.closed.Load() {
		return fmt.Errorf("selected namespace lease is closed")
	}
	if l.record != nil {
		return l.record.Health()
	}
	return nil
}
func (l *Lease) Close() {
	l.once.Do(func() {
		l.closed.Store(true)
		if l.record != nil {
			l.record.Close()
		}
		_ = l.lock.Close()
	})
}
