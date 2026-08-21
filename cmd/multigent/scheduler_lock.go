package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type schedulerStartLock struct {
	path string
}

type schedulerStartLockFile struct {
	Root      string `json:"root"`
	Project   string `json:"project"`
	Agent     string `json:"agent"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

func acquireSchedulerStartLock(root, project, agent string) (*schedulerStartLock, error) {
	root = strings.TrimSpace(root)
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	if root == "" {
		return nil, fmt.Errorf("scheduler lock requires workspace root")
	}
	dir := filepath.Join(root, ".multigent", "scheduler-locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, schedulerStartLockName(root, project, agent)+".json")
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			payload := schedulerStartLockFile{
				Root:      root,
				Project:   project,
				Agent:     agent,
				PID:       os.Getpid(),
				StartedAt: time.Now().UTC().Format(time.RFC3339),
			}
			_ = json.NewEncoder(f).Encode(payload)
			_ = f.Close()
			return &schedulerStartLock{path: path}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		existing, readErr := readSchedulerStartLock(path)
		if readErr == nil && existing.PID > 0 && schedulerProcessAlive(existing.PID) {
			scope := "all projects"
			if project != "" && agent != "" {
				scope = project + "/" + agent
			} else if project != "" {
				scope = "project " + project
			}
			return nil, fmt.Errorf("scheduler already running for %s (pid %d)", scope, existing.PID)
		}
		_ = os.Remove(path)
	}
}

func (l *schedulerStartLock) Release() {
	if l == nil || strings.TrimSpace(l.path) == "" {
		return
	}
	_ = os.Remove(l.path)
}

func schedulerStartLockName(root, project, agent string) string {
	key := strings.Join([]string{root, project, agent}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:12])
}

func readSchedulerStartLock(path string) (schedulerStartLockFile, error) {
	var payload schedulerStartLockFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return payload, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func schedulerProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
