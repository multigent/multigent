//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const runtimeNodeSystemdUnit = runtimeNodeServiceName + ".service"

func servicePlatformName() string {
	if os.Getuid() == 0 {
		return "systemd (system)"
	}
	return "systemd (user)"
}

func runtimeNodeServiceInstallPlatform(cfg runtimeNodeServiceConfig) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl not found: use `multigent runtime start --daemon` instead")
	}
	if err := ensureSystemdAvailableForRuntimeNode(); err != nil {
		return err
	}
	unitPath := runtimeNodeSystemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(runtimeNodeSystemdUnitContent(cfg)), 0o644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	for _, args := range [][]string{
		runtimeNodeSystemctlArgs("daemon-reload"),
		runtimeNodeSystemctlArgs("enable", runtimeNodeSystemdUnit),
		runtimeNodeSystemctlArgs("restart", runtimeNodeSystemdUnit),
	} {
		if out, err := runtimeNodeRunSystemctl(args...); err != nil {
			return fmt.Errorf("systemctl %s: %s (%w)", strings.Join(args, " "), out, err)
		}
	}
	return nil
}

func runtimeNodeServiceUninstallPlatform() error {
	_, _ = runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("disable", "--now", runtimeNodeSystemdUnit)...)
	if err := os.Remove(runtimeNodeSystemdUnitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, _ = runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("daemon-reload")...)
	return nil
}

func runtimeNodeServiceStartPlatform() error {
	out, err := runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("start", runtimeNodeSystemdUnit)...)
	if err != nil {
		return fmt.Errorf("start: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceStopPlatform() error {
	out, err := runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("stop", runtimeNodeSystemdUnit)...)
	if err != nil {
		return fmt.Errorf("stop: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceRestartPlatform() error {
	out, err := runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("restart", runtimeNodeSystemdUnit)...)
	if err != nil {
		return fmt.Errorf("restart: %s (%w)", out, err)
	}
	return nil
}

func runtimeNodeServiceStatusPlatform() (*runtimeNodeServiceStatus, error) {
	st := &runtimeNodeServiceStatus{Supported: true, Platform: servicePlatformName()}
	if _, err := os.Stat(runtimeNodeSystemdUnitPath()); err != nil {
		return st, nil
	}
	st.Installed = true
	out, err := runtimeNodeRunSystemctl(runtimeNodeSystemctlArgs("show", runtimeNodeSystemdUnit, "--no-page", "--property", "ActiveState,MainPID")...)
	if err != nil {
		return st, nil
	}
	props := runtimeNodeParseKeyValue(out)
	if strings.EqualFold(props["ActiveState"], "active") {
		st.Running = true
	}
	if pid, err := strconv.Atoi(props["MainPID"]); err == nil && pid > 0 {
		st.PID = pid
	}
	return st, nil
}

func runtimeNodeSystemctlArgs(args ...string) []string {
	if os.Getuid() == 0 {
		return args
	}
	return append([]string{"--user"}, args...)
}

func runtimeNodeSystemdUnitPath() string {
	if os.Getuid() == 0 {
		return filepath.Join("/etc/systemd/system", runtimeNodeSystemdUnit)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", runtimeNodeSystemdUnit)
}

func runtimeNodeSystemdUnitContent(cfg runtimeNodeServiceConfig) string {
	envPath := cfg.EnvPATH
	if envPath == "" {
		envPath = "/usr/local/bin:/usr/bin:/bin"
	}
	envHome := cfg.EnvHOME
	if envHome == "" {
		envHome = "/"
	}
	var sb strings.Builder
	sb.WriteString("[Unit]\n")
	sb.WriteString("Description=Multigent Runtime Node\n")
	sb.WriteString("After=network-online.target\n")
	sb.WriteString("Wants=network-online.target\n\n")
	sb.WriteString("[Service]\n")
	sb.WriteString("Type=simple\n")
	fmt.Fprintf(&sb, "ExecStart=%s runtime start --poll-interval %s --concurrency %d --log-file %s --log-max-size %d\n",
		strconv.Quote(cfg.BinaryPath), cfg.PollInterval, cfg.Concurrency, strconv.Quote(cfg.LogFile), cfg.LogMaxSizeMB)
	sb.WriteString("Restart=always\n")
	sb.WriteString("RestartSec=5\n")
	fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote("MULTIGENT_RUNTIME_NODE_CONFIG="+cfg.ConfigPath))
	fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote("MULTIGENT_LOG_FILE="+cfg.LogFile))
	fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote(fmt.Sprintf("MULTIGENT_LOG_MAX_SIZE_MB=%d", cfg.LogMaxSizeMB)))
	fmt.Fprintf(&sb, "Environment=MULTIGENT_LOG_STDERR=false\n")
	fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote("PATH="+envPath))
	fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote("HOME="+envHome))
	if strings.TrimSpace(cfg.EnvUSER) != "" {
		fmt.Fprintf(&sb, "Environment=%s\n", strconv.Quote("USER="+cfg.EnvUSER))
	}
	sb.WriteString("\n[Install]\n")
	if os.Getuid() == 0 {
		sb.WriteString("WantedBy=multi-user.target\n")
	} else {
		sb.WriteString("WantedBy=default.target\n")
	}
	return sb.String()
}

func runtimeNodeRunSystemctl(args ...string) (string, error) {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func ensureSystemdAvailableForRuntimeNode() error {
	args := []string{"is-system-running"}
	if os.Getuid() != 0 {
		args = []string{"--user", "is-system-running"}
	}
	out, _ := runtimeNodeRunSystemctl(args...)
	state := strings.TrimSpace(strings.ToLower(out))
	switch state {
	case "running", "degraded", "starting", "initializing":
		return nil
	}
	if runtimeNodeIsWSL2() {
		return fmt.Errorf("systemd is not active in this WSL2 instance. Use `multigent runtime start --daemon`, or enable systemd in /etc/wsl.conf")
	}
	return fmt.Errorf("systemd is not active (state: %s). Use `multigent runtime start --daemon` instead", state)
}

func runtimeNodeIsWSL2() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

func runtimeNodeParseKeyValue(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
