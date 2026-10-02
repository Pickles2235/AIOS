//go:build windows

package main

import "fmt"

func replaceProcess(binary string, args, env []string) error {
	return fmt.Errorf("unsupported upgrade platform")
}
