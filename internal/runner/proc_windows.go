//go:build windows

package runner

import (
	"os/exec"
	"strconv"
)

func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}
