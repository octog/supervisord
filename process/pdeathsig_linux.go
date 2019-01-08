// +build linux

package process

import (
	"syscall"
)

func set_deathsig(sysProcAttr *syscall.SysProcAttr, exitKill bool) {
	// ^C kills all the processes because on Unix typing ^C on the terminal
	// sends a signal to every process in the process group.
	// Setting Setpgid to true means the child process is in a different
	// process group.
	sysProcAttr.Setpgid = true
	// When parent process supervisord exits, it will send sysProcAttr.Pdeathsig
	// to its child processes.
	if exitKill {
		sysProcAttr.Pdeathsig = syscall.SIGKILL
	}
}
