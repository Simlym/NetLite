//go:build windows

package ddnsgo

import (
	"fmt"
	"os/exec"
	"os/user"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

func protectDirectory(path string) error {
	current, err := user.Current()
	if err != nil {
		return fmt.Errorf("cannot identify DDNS configuration owner")
	}
	// Only the current application account inherits access to generated credentials.
	if err = exec.Command("icacls", path, "/inheritance:r", "/grant:r", "*"+current.Uid+":(OI)(CI)F").Run(); err != nil {
		return fmt.Errorf("cannot restrict DDNS directory ACL")
	}
	return nil
}
