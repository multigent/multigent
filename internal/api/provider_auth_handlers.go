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
}

type modelAuthBeginBody struct {
	OwnerType string `json:"ownerType"`
	Name      string `json:"name"`
}

type modelAuthPollBody struct {
	SessionID string `json:"sessionId"`
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
			s.cleanupModelAuthSession(sessionID)
			s.jsonError(w, http.StatusBadGateway, "codex login did not return a device code")
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
			text := string(buf[:n])
			session.mu.Lock()
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
