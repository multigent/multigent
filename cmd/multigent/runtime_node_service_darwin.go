//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const runtimeNodeLaunchdLabel = "dev.multigent.runtime-node"

func servicePlatformName() string { return "launchd" }

func runtimeNodeServiceInstallPlatform(cfg runtimeNodeServiceConfig) error {
	plistPath := runtimeNodeLaunchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), runtimeNodeLaunchdLabel)).Run()
	if err := os.WriteFile(plistPath, []byte(runtimeNodeLaunchdPlist(cfg)), 0o644); err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if out, err := runtimeNodeRunLaunchctl("bootstrap", domain, plistPath); err != nil {
		return fmt.Errorf("launchctl bootstrap: %s (%w)", out, err)
	}
	if out, err := runtimeNodeRunLaunchctl("kickstart", "-kp", fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel)); err != nil {
		return fmt.Errorf("launchctl kickstart: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceUninstallPlatform() error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_, _ = runtimeNodeRunLaunchctl("bootout", fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel))
	if err := os.Remove(runtimeNodeLaunchdPlistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func runtimeNodeServiceStartPlatform() error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	plistPath := runtimeNodeLaunchdPlistPath()
	if out, err := runtimeNodeRunLaunchctl("bootstrap", domain, plistPath); err != nil {
		if out2, err2 := runtimeNodeRunLaunchctl("kickstart", "-kp", fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel)); err2 != nil {
			return fmt.Errorf("start: %s; %s (%w)", out, out2, err2)
		}
	}
	return nil
}

func runtimeNodeServiceStopPlatform() error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	out, err := runtimeNodeRunLaunchctl("bootout", fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel))
	if err != nil {
		return fmt.Errorf("stop: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceRestartPlatform() error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	target := fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel)
	_, _ = runtimeNodeRunLaunchctl("bootout", target)
	plistPath := runtimeNodeLaunchdPlistPath()
	var out string
	var err error
	for i := 0; i < 3; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		out, err = runtimeNodeRunLaunchctl("bootstrap", domain, plistPath)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("restart: %s (%w)", out, err)
	}
	if out, err := runtimeNodeRunLaunchctl("kickstart", "-kp", target); err != nil {
		return fmt.Errorf("restart kickstart: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceStatusPlatform() (*runtimeNodeServiceStatus, error) {
	st := &runtimeNodeServiceStatus{Supported: true, Platform: "launchd"}
	if _, err := os.Stat(runtimeNodeLaunchdPlistPath()); err != nil {
		return st, nil
	}
	st.Installed = true
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	out, _ := runtimeNodeRunLaunchctl("print", fmt.Sprintf("%s/%s", domain, runtimeNodeLaunchdLabel))
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "pid = ") {
			if pid, err := strconv.Atoi(strings.TrimPrefix(trimmed, "pid = ")); err == nil && pid > 0 {
				st.PID = pid
				st.Running = true
			}
		}
		if strings.Contains(trimmed, "state = running") {
			st.Running = true
		}
	}
	return st, nil
}

func runtimeNodeLaunchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", runtimeNodeLaunchdLabel+".plist")
}

func runtimeNodeLaunchdPlist(cfg runtimeNodeServiceConfig) string {
	envPath := cfg.EnvPATH
	if envPath == "" {
		envPath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
	}
	envHome := cfg.EnvHOME
	if envHome == "" {
		envHome, _ = os.UserHomeDir()
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>runtime</string>
		<string>start</string>
		<string>--poll-interval</string>
		<string>%s</string>
		<string>--concurrency</string>
		<string>%d</string>
		<string>--log-file</string>
		<string>%s</string>
		<string>--log-max-size</string>
		<string>%d</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>EnvironmentVariables</key>
	<dict>
		<key>MULTIGENT_RUNTIME_NODE_CONFIG</key>
		<string>%s</string>
		<key>MULTIGENT_LOG_FILE</key>
		<string>%s</string>
		<key>MULTIGENT_LOG_STDERR</key>
		<string>false</string>
		<key>PATH</key>
		<string>%s</string>
		<key>HOME</key>
		<string>%s</string>
		<key>USER</key>
		<string>%s</string>
	</dict>
	<key>StandardOutPath</key>
	<string>/dev/null</string>
	<key>StandardErrorPath</key>
	<string>/dev/null</string>
</dict>
</plist>
`, runtimeNodeLaunchdLabel, cfg.BinaryPath, cfg.PollInterval, cfg.Concurrency, cfg.LogFile, cfg.LogMaxSizeMB, cfg.ConfigPath, cfg.LogFile, envPath, envHome, cfg.EnvUSER)
}

func runtimeNodeRunLaunchctl(args ...string) (string, error) {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
