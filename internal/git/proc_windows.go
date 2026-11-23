//go:build windows

package git

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the process gets a console of its own
// that is never shown.
const createNoWindow = 0x08000000

// detach runs cmd on a console of its own that is not shown: git and the ssh
// it starts cannot write over the screen to ask for a passphrase (Windows has
// no Setsid).
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow} }
