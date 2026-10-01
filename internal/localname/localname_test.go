package localname

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestSelectedNameExclusiveLeaseAndResolution(t *testing.T) {
	name := fmt.Sprintf("aios-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	l, e := Acquire(name)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(l.Close)
	if second, e := Acquire(name); e == nil {
		second.Close()
		t.Fatal("collision accepted")
	}
	if l.Native {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		addresses, e := net.DefaultResolver.LookupIPAddr(ctx, l.Host)
		if e != nil || len(addresses) == 0 {
			t.Fatalf("actual native resolution failed: %v", e)
		}
		for _, a := range addresses {
			if !a.IP.IsLoopback() {
				t.Fatal("native namespace resolved off loopback")
			}
		}
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Fatal("native registration not active")
	}
	l.Close()
	next, e := Acquire(name)
	if e != nil {
		t.Fatal(e)
	}
	next.Close()
}
func TestRejectUnsafeNamespace(t *testing.T) {
	for _, name := range []string{"", "UPPER", "-bad", "bad-", "a.b", "localhost", "local", "../escape", string(make([]byte, 65))} {
		if Validate(name) == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}
