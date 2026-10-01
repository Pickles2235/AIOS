//go:build darwin && arm64 && cgo

package localname

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestNativeOwnershipConflictAcrossConnections(t *testing.T) {
	host := fmt.Sprintf("aios-dns-owner-%d-%d.local", os.Getpid(), time.Now().UnixNano())
	first, err := registerNative(host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	second, err := registerNative(host)
	if err == nil {
		second.Close()
		t.Fatal("independent DNS-SD connection took an already-owned name")
	}
	if err = first.Health(); err != nil {
		t.Fatal("collision displaced original owner", err)
	}
	if err = verifyLoopback(host); err != nil {
		t.Fatal(err)
	}
}
