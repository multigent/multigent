package contextpack

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	CollectorLocalAgentSession = "local-agent-session"

	maxSessionScanBytes   = 256 << 10
	maxSessionImportBytes = 20 << 20
)

type SessionCandidate struct {
	ID         string            `json:"id"`
	CLI        string            `json:"cli"`
	Title      string            `json:"title"`
	Path       string            `json:"path"`
	Size       int64             `json:"size"`
	ModTime    time.Time         `json:"modTime"`
	Message    string            `json:"message,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ImportHint string            `json:"importHint,omitempty"`
}

type SessionScanOptions struct {
	Home  string
	CLI   string
	Limit int
}

func ScanLocalAgentSessions(opts SessionScanOptions) ([]SessionCandidate, error) {
	home := strings.TrimSpace(opts.Home)
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	var candidates []SessionCandidate
	for _, root := range sessionRoots(home, opts.CLI) {
		_ = filepath.WalkDir(root.Path, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".git") || name == "node_modules" || name == "target" || name == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSessionFile(path) {
				return nil
			}
			info, err := d.Info()
			if err != nil || info == nil {
				return nil
			}
			candidate := inspectSessionFile(root.CLI, path, info)
			candidates = append(candidates, candidate)
			return nil
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].ModTime.After(candidates[j].ModTime)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

type sessionRoot struct {
	CLI  string
	Path string
}

func sessionRoots(home, cli string) []sessionRoot {
	cli = strings.TrimSpace(strings.ToLower(cli))
	all := []sessionRoot{
		{CLI: "claudecode", Path: filepath.Join(home, ".claude", "projects")},
		{CLI: "claudecode", Path: filepath.Join(home, ".claude", "sessions")},
		{CLI: "codex", Path: filepath.Join(home, ".codex", "sessions")},
		{CLI: "codex", Path: filepath.Join(home, ".codex", "projects")},
		{CLI: "cursor", Path: filepath.Join(home, ".cursor", "sessions")},
		{CLI: "cursor", Path: filepath.Join(home, ".config", "Cursor", "User", "globalStorage")},
	}
	out := make([]sessionRoot, 0, len(all))
	for _, root := range all {
		if cli != "" && root.CLI != cli {
			continue
		}
		if info, err := os.Stat(root.Path); err == nil && info.IsDir() {
			out = append(out, root)
		}
	}
	return out
}

func isSessionFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jsonl", ".json", ".md", ".txt", ".log":
		return true
	default:
		return false
	}
}

func inspectSessionFile(cli, path string, info os.FileInfo) SessionCandidate {
	title, message, metadata := inferSessionTitle(path)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	sum := sha256.Sum256([]byte(cli + "\x00" + path + "\x00" + info.ModTime().UTC().Format(time.RFC3339Nano)))
	id := "session-" + hex.EncodeToString(sum[:])[:12]
	return SessionCandidate{
		ID:       id,
		CLI:      cli,
		Title:    title,
		Path:     path,
		Size:     info.Size(),
		ModTime:  info.ModTime().UTC(),
		Message:  message,
		Metadata: metadata,
		ImportHint: fmt.Sprintf(
			"multigent context import-session --path %q --cli %s --title %q",
			path,
			cli,
			title,
		),
	}
}

func inferSessionTitle(path string) (string, string, map[string]string) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil
	}
	defer f.Close()
	limited := io.LimitReader(f, maxSessionScanBytes)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64<<10), 512<<10)
	metadata := map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			if strings.HasPrefix(line, "#") {
				return strings.Trim(strings.TrimPrefix(line, "#"), " \t"), "", metadata
			}
			if len(line) > 120 {
				line = line[:120]
			}
			return line, line, metadata
		}
		if sessionID := firstJSONString(obj, "session_id", "sessionId", "id", "conversation_id"); sessionID != "" {
			metadata["sessionId"] = sessionID
		}
		if title := firstJSONString(obj, "title", "threadName", "thread_name", "summary", "name"); title != "" {
			return trimRunes(title, 100), "", metadata
		}
		if msg := messageText(obj); msg != "" {
			return trimRunes(msg, 100), msg, metadata
		}
	}
	return "", "", metadata
}

func firstJSONString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func messageText(obj map[string]any) string {
	if v := firstJSONString(obj, "text", "content", "message"); v != "" {
		return v
	}
	msg, ok := obj["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := msg["content"].([]any)
	if !ok {
		return firstJSONString(msg, "text", "content")
	}
	for _, item := range content {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if text := firstJSONString(part, "text"); text != "" {
			return text
		}
	}
	return ""
}

func trimRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "..."
}
