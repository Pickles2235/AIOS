//go:build !darwin || !arm64 || !cgo

package localname

import "fmt"

func registerNative(string) (nativeRecord, error) {
	return nil, fmt.Errorf("native DNS-SD requires Apple Silicon and bundled native support")
}
