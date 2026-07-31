package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) saasProxyUser(r *http.Request) (string, bool) {
	secret := strings.TrimSpace(os.Getenv("MULTIGENT_SAAS_TRUST_SECRET"))
	if secret == "" {
		return "", false
	}
	userID := strings.TrimSpace(r.Header.Get(saasUserIDHeader))
	email := normalizeEmail(r.Header.Get(saasUserEmailHeader))
	workspaceID := strings.TrimSpace(r.Header.Get(saasWorkspaceIDHeader))
	ts := strings.TrimSpace(r.Header.Get(saasTimestampHeader))
	sig := strings.TrimSpace(r.Header.Get(saasSignatureHeader))
	if userID == "" || email == "" || workspaceID == "" || ts == "" || sig == "" {
		return "", false
	}
	parsed, err := parseSaaSTimestamp(ts)
	if err != nil || time.Since(parsed) > 5*time.Minute || time.Until(parsed) > 5*time.Minute {
		return "", false
	}
	msg := strings.Join([]string{ts, userID, email, workspaceID}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return "", false
	}
	username, err := s.ensureSaaSUser(email, strings.TrimSpace(r.Header.Get(saasUserNameHeader)))
	if err != nil {
		return "", false
	}
	if s.controlDB != nil {
		if currentWorkspaceID, err := s.currentWorkspaceID(); err == nil {
			_ = s.ensureCurrentUserMembership(currentWorkspaceID, username)
		}
	}
	return username, true
}

func parseSaaSTimestamp(raw string) (time.Time, error) {
	var unix int64
	if _, err := fmt.Sscanf(raw, "%d", &unix); err != nil {
		return time.Time{}, err
	}
	return time.Unix(unix, 0), nil
}

func (s *Server) ensureSaaSUser(email, displayName string) (string, error) {
	if user := s.users.UserByEmail(email); user != nil {
		if displayName != "" && user.DisplayName == "" {
			next := displayName
			if err := s.users.UpdateUser(user.Username, nil, &next, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
				return user.Username, nil
			}
		}
		return user.Username, nil
	}
	username := s.users.uniqueUsernameLocked(usernameFromEmail(email))
	hash, err := bcrypt.GenerateFromPassword([]byte(generateSecret()), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if displayName == "" {
		displayName = saasDisplayName(email)
	}
	err = s.controlDB.UpsertUser(controldb.User{
		Username:     username,
		Role:         RoleAdmin,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Email:        email,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	return username, err
}

func saasDisplayName(email string) string {
	local := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	if local == "" {
		return "SaaS User"
	}
	return local
}
