//go:build !windows

package main

import "syscall"

func replaceProcess(binary string, args, env []string) error { return syscall.Exec(binary, args, env) }
