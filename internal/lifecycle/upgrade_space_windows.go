//go:build windows

package lifecycle

import "fmt"

func checkUpgradeSpace(path string, required uint64) error {
	return fmt.Errorf("unsupported upgrade platform")
}
