package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"golang.org/x/crypto/bcrypt"
)

var errIdentityForbidden = errors.New("account disabled")

type identitySource string

const (
	identitySourceLocalToken   identitySource = "local_token"
	identitySourceStaticAPIKey identitySource = "static_api_key"
	identitySourceTrustedProxy identitySource = "trusted_proxy"
)

type authenticatedIdentity struct {
	Username string
	Source   identitySource
	Err      error
}

type identityProvider interface {
	Authenticate(*http.Request) (authenticatedIdentity, bool)
}

func (s *Server) authenticateRequest(r *http.Request) (authenticatedIdentity, bool) {
	for _, provider := range []identityProvider{
		trustedProxyIdentityProvider{server: s},
		localTokenIdentityProvider{server: s},
	} {
		identity, handled := provider.Authenticate(r)
		if handled {
			return identity, true
		}
	}
	return authenticatedIdentity{}, false
}

type localTokenIdentityProvider struct {
	server *Server
}

func (p localTokenIdentityProvider) Authenticate(r *http.Request) (authenticatedIdentity, bool) {
	token := bearerOrQueryToken(r)
	if token == "" {
		return authenticatedIdentity{}, false
	}
	if p.server.apiKey != "" && token == p.server.apiKey {
		return authenticatedIdentity{Username: "apikey", Source: identitySourceStaticAPIKey}, true
	}
	username, ok := p.server.users.ValidateToken(token)
	if !ok {
		return authenticatedIdentity{Err: errors.New("invalid or expired token")}, true
	}
	if u := p.server.users.GetUser(username); u != nil && u.Disabled {
		return authenticatedIdentity{Err: errIdentityForbidden}, true
	}
	return authenticatedIdentity{Username: username, Source: identitySourceLocalToken}, true
}

func bearerOrQueryToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return strings.TrimSpace(r.URL.Query().Get("_token"))
}

type trustedProxyIdentityProvider struct {
	server *Server
}

func (p trustedProxyIdentityProvider) Authenticate(r *http.Request) (authenticatedIdentity, bool) {
	secret := strings.TrimSpace(os.Getenv("MULTIGENT_TRUSTED_PROXY_SECRET"))
	if secret == "" {
		return authenticatedIdentity{}, false
	}
	userID := strings.TrimSpace(r.Header.Get(trustedProxyUserIDHeader))
	email := normalizeEmail(r.Header.Get(trustedProxyUserEmailHeader))
	workspaceID := strings.TrimSpace(r.Header.Get(trustedProxyWorkspaceIDHeader))
	ts := strings.TrimSpace(r.Header.Get(trustedProxyTimestampHeader))
	sig := strings.TrimSpace(r.Header.Get(trustedProxySignatureHeader))
	if userID == "" || email == "" || workspaceID == "" || ts == "" || sig == "" {
		return authenticatedIdentity{}, false
	}
	parsed, err := parseTrustedProxyTimestamp(ts)
	if err != nil || time.Since(parsed) > 5*time.Minute || time.Until(parsed) > 5*time.Minute {
		return authenticatedIdentity{Err: errors.New("invalid trusted proxy timestamp")}, true
	}
	msg := strings.Join([]string{ts, userID, email, workspaceID}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return authenticatedIdentity{Err: errors.New("invalid trusted proxy signature")}, true
	}
	username, err := p.server.ensureTrustedProxyUser(email, strings.TrimSpace(r.Header.Get(trustedProxyUserNameHeader)))
	if err != nil {
		return authenticatedIdentity{Err: err}, true
	}
	if p.server.controlDB != nil {
		if currentWorkspaceID, err := p.server.currentWorkspaceID(); err == nil {
			_ = p.server.ensureCurrentUserMembership(currentWorkspaceID, username)
		}
	}
	return authenticatedIdentity{Username: username, Source: identitySourceTrustedProxy}, true
}

func parseTrustedProxyTimestamp(raw string) (time.Time, error) {
	var unix int64
	if _, err := fmt.Sscanf(raw, "%d", &unix); err != nil {
		return time.Time{}, err
	}
	return time.Unix(unix, 0), nil
}

func (s *Server) ensureTrustedProxyUser(email, displayName string) (string, error) {
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
		displayName = trustedProxyDisplayName(email)
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

func trustedProxyDisplayName(email string) string {
	local := strings.TrimSpace(strings.SplitN(email, "@", 2)[0])
	if local == "" {
		return "External User"
	}
	return local
}
