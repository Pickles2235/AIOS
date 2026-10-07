//go:build windows

package observability

import "fmt"

// The installable product is macOS Apple Silicon. Windows builds retain the
// API but never create a weaker telemetry sink.
type secureRoot struct{}

func openRoot(string) (*secureRoot, error) {
	return nil, fmt.Errorf("local diagnostic sink requires macOS or Unix")
}
func openExistingRoot(string) (*secureRoot, error) {
	return nil, fmt.Errorf("diagnostic sink unavailable")
}
func (*secureRoot) close() error                  { return nil }
func (*secureRoot) append([]byte, int, int) error { return fmt.Errorf("diagnostic sink unavailable") }
func (*secureRoot) read(int, int) ([]byte, error) {
	return nil, fmt.Errorf("diagnostic sink unavailable")
}
