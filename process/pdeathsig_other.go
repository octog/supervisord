// +build !linux
// +build !windows

package process

import (
	"syscall"
)

func set_deathsig(sysProcAttr *syscall.SysProcAttr, _ bool) {
	sysProcAttr.Setpgid = true
}
