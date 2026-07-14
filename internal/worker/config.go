package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Mode string

const (
	ModeImport          Mode = "import"
	ModePrivateResource Mode = "private-resource"
	ModeEnterprise      Mode = "enterprise"
)

func (m Mode) String() string {
	return string(normaliseMode(string(m)))
}

func (m *Mode) Set(value string) error {
	mode := normaliseMode(value)
	if strings.TrimSpace(value) != "" && mode == ModeImport && strings.TrimSpace(value) != string(ModeImport) {
		return fmt.Errorf("unsupported worker mode %q; use import, private-resource, or enterprise", value)
	}
	*m = mode
	return nil
}

func (m Mode) Type() string {
	return "worker-mode"
}

type Config struct {
	Mode            Mode          `json:"mode"`
	WorkerID        string        `json:"worker_id"`
	ControlPlaneURL string        `json:"control_plane_url,omitempty"`
	Token           string        `json:"-"`
	Workspace       string        `json:"workspace,omitempty"`
	PollInterval    time.Duration `json:"poll_interval"`
	Capacity        int           `json:"capacity"`
}

func FromEnv() Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "local"
	}
	return Config{
		Mode:            normaliseMode(os.Getenv("MULTIGENT_WORKER_MODE")),
		WorkerID:        firstNonEmpty(os.Getenv("MULTIGENT_WORKER_ID"), hostname),
		ControlPlaneURL: os.Getenv("MULTIGENT_CONTROL_PLANE_URL"),
		Token:           os.Getenv("MULTIGENT_WORKER_TOKEN"),
		Workspace:       os.Getenv("MULTIGENT_WORKER_WORKSPACE"),
		PollInterval:    10 * time.Second,
		Capacity:        1,
	}
}

func (c Config) ValidateForStart() error {
	if c.WorkerID == "" {
		return fmt.Errorf("worker id is required")
	}
	mode := normaliseMode(string(c.Mode))
	if mode == ModeImport {
		return fmt.Errorf("import mode does not start a long-running worker; use `multigent worker import scan --path <agent-dir>`")
	}
	if mode != ModePrivateResource && mode != ModeEnterprise {
		return fmt.Errorf("unsupported worker mode %q; use import, private-resource, or enterprise", c.Mode)
	}
	if c.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL is required; set --control-plane-url or MULTIGENT_CONTROL_PLANE_URL")
	}
	if c.Token == "" {
		return fmt.Errorf("worker token is required; set --token or MULTIGENT_WORKER_TOKEN")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	if c.Capacity <= 0 {
		return fmt.Errorf("capacity must be positive")
	}
	return nil
}

func (c Config) MarshalJSONPretty() ([]byte, error) {
	type wire struct {
		Mode            Mode   `json:"mode"`
		WorkerID        string `json:"worker_id"`
		ControlPlaneURL string `json:"control_plane_url,omitempty"`
		TokenSet        bool   `json:"token_set"`
		Workspace       string `json:"workspace,omitempty"`
		PollInterval    string `json:"poll_interval"`
		Capacity        int    `json:"capacity"`
	}
	return json.MarshalIndent(wire{
		Mode:            normaliseMode(string(c.Mode)),
		WorkerID:        c.WorkerID,
		ControlPlaneURL: c.ControlPlaneURL,
		TokenSet:        c.Token != "",
		Workspace:       c.Workspace,
		PollInterval:    c.PollInterval.String(),
		Capacity:        c.Capacity,
	}, "", "  ")
}

func normaliseMode(mode string) Mode {
	switch Mode(strings.TrimSpace(mode)) {
	case ModePrivateResource:
		return ModePrivateResource
	case ModeEnterprise:
		return ModeEnterprise
	default:
		return ModeImport
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
