// +build !windows

package signals

import (
	"os"
	"strings"
	"syscall"
)

//convert a signal name to signal
func ToSignal(signalName string) (os.Signal, error) {
	signalName = strings.ToUpper(signalName)
	if signalName == "HUP" {
		return syscall.SIGHUP, nil
	} else if signalName == "INT" {
		return syscall.SIGINT, nil
	} else if signalName == "QUIT" {
		return syscall.SIGQUIT, nil
	} else if signalName == "KILL" {
		return syscall.SIGKILL, nil
	} else if signalName == "USR1" {
		return syscall.SIGUSR1, nil
	} else if signalName == "USR2" {
		return syscall.SIGUSR2, nil
	}

	return syscall.SIGTERM, nil
}

// send signal to the proces
//
// Args:
//    process - the process which the signal should be sent to
//    sig - the signal will be sent
//    sigChildren - true if the signal needs to be sent to the children also
// 注意负号,　杀掉包括子进程
func Kill(process *os.Process, sig os.Signal, sigChildren bool) error {
	return KillPid(-process.Pid, sig, sigChildren)
}

func KillPid(pid int, sig os.Signal, sigChildren bool) error {
	localSig := sig.(syscall.Signal)
	if sigChildren {
		pid = -pid
	}
	return syscall.Kill(pid, localSig)
}
