package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

const codexDeviceAuthURL = "https://auth.openai.com/codex/device"

var codexDeviceCodePattern = regexp.MustCompile(`\b[A-Z0-9]{4}-[A-Z0-9]{4}[A-Z0-9-]*\b`)
var authURLPattern = regexp.MustCompile(`https?://[^\s\x1b]+`)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

type modelAuthSession struct {
	mu              sync.Mutex
	ID              string
	CLI             string
	Name            string
	OwnerType       string
	HomeDir         string
	CreatedAt       time.Time
	Cancel          context.CancelFunc
	TTY             *os.File
	VerificationURI string
	UserCode        string
	Ready           bool
	Done            bool
	ExitErr         string
	ProviderID      string
	Output          string
}

type modelAuthBeginBody struct {
	OwnerType string `json:"ownerType"`
	Name      string `json:"name"`
}

type modelAuthPollBody struct {
	SessionID string `json:"sessionId"`
}

type modelAuthCodeBody struct {
	SessionID string `json:"sessionId"`
	Code      string `json:"code"`
}

func (s *Server) handleCodexDeviceAuthBegin(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	var body modelAuthBeginBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prov := entity.APIProvider{
		OwnerType: strings.TrimSpace(body.OwnerType),
		Name:      strings.TrimSpace(body.Name),
		Type:      "openai",
		Env: map[string]string{
			store.ProviderAuthMethodEnvKey: store.ProviderAuthMethodCodexChatGPT,
			store.ProviderAuthStatusEnvKey: store.ProviderAuthStatusNotConfigured,
		},
	}
	if prov.Name == "" {
		prov.Name = "Codex ChatGPT"
	}
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	sessionID := "auth-" + randomHex(12)
	homeDir := filepath.Join(s.root, ".multigent", "model-auth-sessions", sessionID)
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		s.serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	cmd := exec.CommandContext(ctx, "codex", "login", "--device-auth")
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"))
	tty, err := pty.Start(cmd)
	if err != nil {
		cancel()
		_ = os.RemoveAll(homeDir)
		s.jsonError(w, http.StatusBadGateway, "failed to start codex login")
		return
	}
	session := &modelAuthSession{
		ID:        sessionID,
		CLI:       "codex",
		Name:      prov.Name,
		OwnerType: prov.OwnerType,
		HomeDir:   homeDir,
		CreatedAt: time.Now(),
		Cancel:    cancel,
		TTY:       tty,
	}
	s.modelAuthMu.Lock()
	s.modelAuthSessions[sessionID] = session
	s.modelAuthMu.Unlock()
	go runCodexDeviceAuthSession(cmd, tty, session)

	deadline := time.After(8 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			session.mu.Lock()
			output := session.Output
			session.mu.Unlock()
			s.cleanupModelAuthSession(sessionID)
			s.jsonError(w, http.StatusBadGateway, "codex login did not return a device code: "+shortAuthOutput(output))
			return
		case <-tick.C:
			session.mu.Lock()
			ready, done, exitErr := session.Ready, session.Done, session.ExitErr
			uri, code := session.VerificationURI, session.UserCode
			session.mu.Unlock()
			if ready {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"sessionId":       sessionID,
					"verificationUri": uri,
					"userCode":        code,
					"expiresIn":       900,
					"status":          "pending",
				})
				return
			}
			if done {
				if exitErr == "" {
					exitErr = "codex login exited before producing a device code"
				}
				s.cleanupModelAuthSession(sessionID)
				s.jsonError(w, http.StatusBadGateway, exitErr)
				return
			}
		}
	}
}

func (s *Server) handleCodexDeviceAuthPoll(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	var body modelAuthPollBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	s.modelAuthMu.Lock()
	session := s.modelAuthSessions[sessionID]
	s.modelAuthMu.Unlock()
	if session == nil {
		s.jsonError(w, http.StatusNotFound, "auth session not found")
		return
	}
	session.mu.Lock()
	done, exitErr, providerID := session.Done, session.ExitErr, session.ProviderID
	session.mu.Unlock()
	if providerID != "" {
		p, err := s.providerStore().Get(providerID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "connected", "provider": providerToJSON(*p)})
		return
	}
	if !done {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		return
	}
	if exitErr != "" {
		s.cleanupModelAuthSession(sessionID)
		s.jsonError(w, http.StatusBadGateway, exitErr)
		return
	}
	authJSON := filepath.Join(session.HomeDir, ".codex", "auth.json")
	if !modelAuthFileExists(authJSON) {
		s.cleanupModelAuthSession(sessionID)
		s.jsonError(w, http.StatusBadGateway, "codex login completed but auth.json was not created")
		return
	}
	prov := entity.APIProvider{
		OwnerType: session.OwnerType,
		Name:      session.Name,
		Type:      "openai",
		Env: map[string]string{
			store.ProviderAuthMethodEnvKey: store.ProviderAuthMethodCodexChatGPT,
			store.ProviderAuthStatusEnvKey: store.ProviderAuthStatusConfigured,
		},
	}
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	p, err := s.providerStore().Add(prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	dst := filepath.Join(store.ProviderCredentialDir(s.root, p.ID, entity.ModelCodex), ".codex", "auth.json")
	if err := copyFile0600(authJSON, dst); err != nil {
		_ = s.providerStore().Remove(p.ID)
		s.serverError(w, err)
		return
	}
	session.mu.Lock()
	session.ProviderID = p.ID
	session.mu.Unlock()
	s.cleanupModelAuthSession(sessionID)
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.create",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      "Codex ChatGPT model provider connected",
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "connected", "provider": providerToJSON(*p)})
}

func (s *Server) handleCLIBrowserAuthBegin(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	spec, ok := browserAuthSpec(r.PathValue("cli"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported CLI browser auth")
		return
	}
	var body modelAuthBeginBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prov := entity.APIProvider{
		OwnerType: strings.TrimSpace(body.OwnerType),
		Name:      strings.TrimSpace(body.Name),
		Type:      spec.ProviderType,
		Env: map[string]string{
			store.ProviderAuthMethodEnvKey: spec.AuthMethod,
			store.ProviderAuthStatusEnvKey: store.ProviderAuthStatusNotConfigured,
		},
	}
	if prov.Name == "" {
		prov.Name = spec.DefaultName
	}
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	sessionID := "auth-" + randomHex(12)
	homeDir := filepath.Join(s.root, ".multigent", "model-auth-sessions", sessionID)
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		s.serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	cmd := exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "XDG_CONFIG_HOME="+filepath.Join(homeDir, ".config"))
	for key, value := range spec.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	tty, err := pty.Start(cmd)
	if err != nil {
		cancel()
		_ = os.RemoveAll(homeDir)
		s.jsonError(w, http.StatusBadGateway, "failed to start CLI login")
		return
	}
	session := &modelAuthSession{
		ID:        sessionID,
		CLI:       spec.CLI,
		Name:      prov.Name,
		OwnerType: prov.OwnerType,
		HomeDir:   homeDir,
		CreatedAt: time.Now(),
		Cancel:    cancel,
		TTY:       tty,
	}
	s.modelAuthMu.Lock()
	s.modelAuthSessions[sessionID] = session
	s.modelAuthMu.Unlock()
	go runBrowserAuthSession(cmd, tty, session)

	deadline := time.After(8 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			s.cleanupModelAuthSession(sessionID)
			s.jsonError(w, http.StatusBadGateway, "CLI login did not return an authorization URL")
			return
		case <-tick.C:
			session.mu.Lock()
			ready, done, exitErr, uri := session.Ready, session.Done, session.ExitErr, session.VerificationURI
			session.mu.Unlock()
			if ready {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"sessionId":       sessionID,
					"verificationUri": uri,
					"status":          "pending",
					"requiresCode":    spec.RequiresCode,
				})
				return
			}
			if done {
				if exitErr == "" {
					exitErr = "CLI login exited before producing an authorization URL"
				}
				s.cleanupModelAuthSession(sessionID)
				s.jsonError(w, http.StatusBadGateway, exitErr)
				return
			}
		}
	}
}

func (s *Server) handleCLIBrowserAuthPoll(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceMember(w, r)
	if err != nil {
		return
	}
	spec, ok := browserAuthSpec(r.PathValue("cli"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported CLI browser auth")
		return
	}
	var body modelAuthPollBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session := s.getModelAuthSession(strings.TrimSpace(body.SessionID))
	if session == nil || session.CLI != spec.CLI {
		s.jsonError(w, http.StatusNotFound, "auth session not found")
		return
	}
	session.mu.Lock()
	done, exitErr, providerID := session.Done, session.ExitErr, session.ProviderID
	session.mu.Unlock()
	if providerID != "" {
		p, err := s.providerStore().Get(providerID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "connected", "provider": providerToJSON(*p)})
		return
	}
	if !done {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending", "requiresCode": spec.RequiresCode})
		return
	}
	if exitErr != "" {
		s.cleanupModelAuthSession(session.ID)
		s.jsonError(w, http.StatusBadGateway, exitErr)
		return
	}
	if !spec.CredentialsReady(session.HomeDir) {
		s.cleanupModelAuthSession(session.ID)
		s.jsonError(w, http.StatusBadGateway, "CLI login completed but credential files were not created")
		return
	}
	prov := entity.APIProvider{
		OwnerType: session.OwnerType,
		Name:      session.Name,
		Type:      spec.ProviderType,
		Env: map[string]string{
			store.ProviderAuthMethodEnvKey: spec.AuthMethod,
			store.ProviderAuthStatusEnvKey: store.ProviderAuthStatusConfigured,
		},
	}
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	p, err := s.providerStore().Add(prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := spec.CopyCredentials(session.HomeDir, store.ProviderCredentialDir(s.root, p.ID, spec.Model)); err != nil {
		_ = s.providerStore().Remove(p.ID)
		s.serverError(w, err)
		return
	}
	session.mu.Lock()
	session.ProviderID = p.ID
	session.mu.Unlock()
	s.cleanupModelAuthSession(session.ID)
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.create",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      spec.DefaultName + " model provider connected",
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "connected", "provider": providerToJSON(*p)})
}

func (s *Server) handleCLIBrowserAuthCode(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	spec, ok := browserAuthSpec(r.PathValue("cli"))
	if !ok {
		s.jsonError(w, http.StatusBadRequest, "unsupported CLI browser auth")
		return
	}
	var body modelAuthCodeBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	session := s.getModelAuthSession(strings.TrimSpace(body.SessionID))
	if session == nil || session.CLI != spec.CLI {
		s.jsonError(w, http.StatusNotFound, "auth session not found")
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		s.jsonError(w, http.StatusBadRequest, "code is required")
		return
	}
	session.mu.Lock()
	tty := session.TTY
	session.mu.Unlock()
	if tty == nil {
		s.jsonError(w, http.StatusConflict, "auth session is not writable")
		return
	}
	if _, err := tty.WriteString(code + "\n"); err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleModelAuthSessionCancel(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if sessionID == "" {
		s.jsonError(w, http.StatusBadRequest, "session id is required")
		return
	}
	s.cleanupModelAuthSession(sessionID)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func runCodexDeviceAuthSession(cmd *exec.Cmd, tty *os.File, session *modelAuthSession) {
	defer tty.Close()
	buf := make([]byte, 4096)
	for {
		n, readErr := tty.Read(buf)
		if n > 0 {
			text := stripTerminalControls(string(buf[:n]))
			session.mu.Lock()
			session.Output += text
			if strings.Contains(text, codexDeviceAuthURL) {
				session.VerificationURI = codexDeviceAuthURL
			}
			if session.UserCode == "" {
				if code := codexDeviceCodePattern.FindString(text); code != "" && code != "GPT5-CODEX" {
					session.UserCode = code
				}
			}
			if session.VerificationURI != "" && session.UserCode != "" {
				session.Ready = true
			}
			session.mu.Unlock()
		}
		if readErr != nil {
			break
		}
	}
	err := cmd.Wait()
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Done = true
	if err != nil {
		session.ExitErr = fmt.Sprintf("codex login failed: %v", err)
	}
}

func (s *Server) getModelAuthSession(sessionID string) *modelAuthSession {
	s.modelAuthMu.Lock()
	defer s.modelAuthMu.Unlock()
	return s.modelAuthSessions[sessionID]
}

func runBrowserAuthSession(cmd *exec.Cmd, tty *os.File, session *modelAuthSession) {
	defer tty.Close()
	buf := make([]byte, 4096)
	for {
		n, readErr := tty.Read(buf)
		if n > 0 {
			text := stripTerminalControls(string(buf[:n]))
			session.mu.Lock()
			session.Output += text
			if session.VerificationURI == "" {
				if uri := firstCleanAuthURL(text); uri != "" {
					session.VerificationURI = uri
					session.Ready = true
				}
			}
			session.mu.Unlock()
		}
		if readErr != nil {
			break
		}
	}
	err := cmd.Wait()
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Done = true
	if err != nil {
		session.ExitErr = fmt.Sprintf("CLI login failed: %v", err)
	}
}

type cliBrowserAuthSpec struct {
	CLI              string
	DefaultName      string
	ProviderType     string
	AuthMethod       string
	Model            entity.AgentModel
	Command          []string
	Env              map[string]string
	RequiresCode     bool
	CredentialsReady func(home string) bool
	CopyCredentials  func(home, dstRoot string) error
}

func browserAuthSpec(cli string) (cliBrowserAuthSpec, bool) {
	switch strings.ToLower(strings.TrimSpace(cli)) {
	case "claudecode", "claude":
		return cliBrowserAuthSpec{
			CLI:          "claudecode",
			DefaultName:  "Claude Code Browser",
			ProviderType: "anthropic",
			AuthMethod:   store.ProviderAuthMethodClaudeBrowser,
			Model:        entity.ModelClaudeCode,
			Command:      []string{"claude", "auth", "login", "--claudeai"},
			RequiresCode: true,
			CredentialsReady: func(home string) bool {
				return modelAuthFileExists(filepath.Join(home, ".claude.json")) ||
					modelAuthFileExists(filepath.Join(home, ".claude", ".credentials.json"))
			},
			CopyCredentials: copyClaudeBrowserCredentials,
		}, true
	case "cursor":
		return cliBrowserAuthSpec{
			CLI:          "cursor",
			DefaultName:  "Cursor Browser",
			ProviderType: "cursor",
			AuthMethod:   store.ProviderAuthMethodCursorBrowser,
			Model:        entity.ModelCursor,
			Command:      []string{"agent", "login"},
			Env:          map[string]string{"NO_OPEN_BROWSER": "1"},
			CredentialsReady: func(home string) bool {
				return modelAuthFileExists(filepath.Join(home, ".cursor", "cli-config.json"))
			},
			CopyCredentials: copyCursorBrowserCredentials,
		}, true
	default:
		return cliBrowserAuthSpec{}, false
	}
}

func firstCleanAuthURL(text string) string {
	for _, match := range authURLPattern.FindAllString(text, -1) {
		if cleaned := cleanTerminalURL(match); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func stripTerminalControls(text string) string {
	text = ansiEscapePattern.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func shortAuthOutput(text string) string {
	text = strings.TrimSpace(stripTerminalControls(text))
	if text == "" {
		return "no output"
	}
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 600 {
		return text[:600] + "..."
	}
	return text
}

func cleanTerminalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.IndexByte(raw, '\x1b'); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimRight(raw, "\a\"')],.;")
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return ""
	}
	return raw
}

func copyClaudeBrowserCredentials(home, dstRoot string) error {
	for _, rel := range []string{".claude.json", filepath.Join(".claude", ".credentials.json")} {
		src := filepath.Join(home, rel)
		if modelAuthFileExists(src) {
			if err := copyFile0600(src, filepath.Join(dstRoot, rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyCursorBrowserCredentials(home, dstRoot string) error {
	for _, rel := range []string{filepath.Join(".cursor", "cli-config.json")} {
		src := filepath.Join(home, rel)
		if modelAuthFileExists(src) {
			if err := copyFile0600(src, filepath.Join(dstRoot, rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) cleanupModelAuthSession(sessionID string) {
	s.modelAuthMu.Lock()
	session := s.modelAuthSessions[sessionID]
	delete(s.modelAuthSessions, sessionID)
	s.modelAuthMu.Unlock()
	if session == nil {
		return
	}
	if session.Cancel != nil {
		session.Cancel()
	}
	if session.TTY != nil {
		_ = session.TTY.Close()
	}
	_ = os.RemoveAll(session.HomeDir)
}

func randomHex(n int) string {
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func copyFile0600(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func modelAuthFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
