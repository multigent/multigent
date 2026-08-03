//go:build windows

package main

import "os/exec"

func setBackgroundProcessGroup(cmd *exec.Cmd) {}
