//go:build windows

package cli

import (
	"os"
	"os/exec"
	"strconv"
)

func setProcessGroup(cmd *exec.Cmd) {}

func terminateProcess(p *os.Process) {
	if p == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run()
}

func killProcess(p *os.Process) {
	if p == nil {
		return
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run(); err != nil {
		_ = p.Kill()
	}
}
