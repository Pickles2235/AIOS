//go:build !darwin && !linux

package adapter

import (
	"fmt"
	"os"
)

func openWorkingFile(root *os.Root, path string) (*os.File, error) {
	return nil, fmt.Errorf("working-tree capture requires supported native or Linux developer platform")
}
