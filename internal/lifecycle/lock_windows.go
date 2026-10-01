package lifecycle

import (
	"fmt"
	"os"
)

func Lock(data string) (*os.File, error) {
	return nil, fmt.Errorf("daemon control is supported on macOS Apple Silicon; Linux is developer smoke")
}
func owned(info os.FileInfo) bool             { return false }
func openOwned(path string) (*os.File, error) { return nil, fmt.Errorf("unsupported daemon platform") }
