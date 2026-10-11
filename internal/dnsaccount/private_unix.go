//go:build !windows

package dnsaccount

import (
	"os"
)

func protect(path string) error { return os.Chmod(path, 0700) }
