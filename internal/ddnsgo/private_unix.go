//go:build !windows

package ddnsgo

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd) {}

func protectDirectory(path string) error { return os.Chmod(path, 0700) }
