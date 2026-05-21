//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func terminateProcess(p *os.Process) {
	if p == nil {
		return
	}
	pgid, err := syscall.Getpgid(p.Pid)
	if err != nil {
		pgid = p.Pid
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
}

func killProcess(p *os.Process) {
	if p == nil {
		return
	}
	pgid, err := syscall.Getpgid(p.Pid)
	if err != nil {
		pgid = p.Pid
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
