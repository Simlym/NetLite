//go:build windows

package dnsaccount

import (
	"fmt"
	"os/exec"
	"os/user"
)

func protect(path string) error {
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
