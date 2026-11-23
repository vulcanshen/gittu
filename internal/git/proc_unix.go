//go:build !windows

package git

import (
	"os/exec"
	"syscall"
)

// detach runs cmd in a session of its own: no controlling terminal, so ssh
// and gpg cannot open /dev/tty and write over the screen to ask for a
// passphrase.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
