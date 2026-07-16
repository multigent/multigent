package appconfig

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Workspace WorkspaceConfig
	Server    ServerConfig
	Auth      AuthConfig
	SMTP      SMTPConfig
	Logging   LoggingConfig
	Sandbox   SandboxConfig
}

type WorkspaceConfig struct {
	Dir string
}

type ServerConfig struct {
	Addr string
}

type AuthConfig struct {
	APIKey string
}

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	TLS      string
}

type LoggingConfig struct {
	File      string
	Level     string
	Format    string
	MaxSizeMB int
	Stderr    *bool
}

type SandboxConfig struct {
	E2B E2BConfig
}

type E2BConfig struct {
	APIURL string
}

func Load(path string) (*Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return &Config{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{}
	section := ""
	sc := bufio.NewScanner(f)
	for lineNo := 1; sc.Scan(); lineNo++ {
		line := stripComment(strings.TrimSpace(sc.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.Trim(line, "[]"))
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		if err := setValue(cfg, section, strings.TrimSpace(key), strings.TrimSpace(raw)); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func setValue(cfg *Config, section, key, raw string) error {
	switch section {
	case "workspace":
		if key == "dir" {
			cfg.Workspace.Dir = stringValue(raw)
		}
	case "server":
		if key == "addr" {
			cfg.Server.Addr = stringValue(raw)
		}
	case "auth":
		if key == "api_key" {
			cfg.Auth.APIKey = stringValue(raw)
		}
	case "smtp":
		switch key {
		case "host":
			cfg.SMTP.Host = stringValue(raw)
		case "port":
			cfg.SMTP.Port = intValue(raw)
		case "username":
			cfg.SMTP.Username = stringValue(raw)
		case "password":
			cfg.SMTP.Password = stringValue(raw)
		case "from":
			cfg.SMTP.From = stringValue(raw)
		case "from_name":
			cfg.SMTP.FromName = stringValue(raw)
		case "tls":
			cfg.SMTP.TLS = stringValue(raw)
		}
	case "logging":
		switch key {
		case "file":
			cfg.Logging.File = stringValue(raw)
		case "level":
			cfg.Logging.Level = stringValue(raw)
		case "format":
			cfg.Logging.Format = stringValue(raw)
		case "max_size_mb":
			cfg.Logging.MaxSizeMB = intValue(raw)
		case "stderr":
			v := boolValue(raw)
			cfg.Logging.Stderr = &v
		}
	case "sandbox.e2b":
		if key == "api_url" {
			cfg.Sandbox.E2B.APIURL = stringValue(raw)
		}
	}
	return nil
}

func stripComment(s string) string {
	inString := false
	for i, r := range s {
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '#' && !inString {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

func stringValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		if v, err := strconv.Unquote(raw); err == nil {
			return v
		}
		return strings.Trim(raw, `"`)
	}
	return raw
}

func intValue(raw string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(raw))
	return v
}

func boolValue(raw string) bool {
	v, _ := strconv.ParseBool(strings.TrimSpace(raw))
	return v
}
