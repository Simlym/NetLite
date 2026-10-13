//go:build linux

package main

import (
	"errors"
	"syscall"
)

func isStaleSocket(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
