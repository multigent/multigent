package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	clientTokensTable    = "client_tokens"
	clientTokenPrefix    = "mgpat_"
	clientScopeContextRW = "context.write"
)

type clientTokenRecord struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"tokenHash,omitempty"`
	Username    string     `json:"username"`
	Scopes      []string   `json:"scopes"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	Description string     `json:"description,omitempty"`
}

type createClientTokenBody struct {
	Name        string   `json:"name"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   string   `json:"expiresAt"`
	Description string   `json:"description"`
}

type createClientTokenResponse struct {
	Token clientTokenRecord `json:"token"`
	Raw   string            `json:"rawToken"`
}

func (s *Server) handleClientTokensList(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	tokens, err := s.listClientTokens(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"tokens": redactClientTokenHashes(filterClientTokensForUser(tokens, cur.Username))})
}

func (s *Server) handleClientTokensCreate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	var body createClientTokenBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "token name is required")
		return
	}
	scopes := normalizeClientTokenScopes(body.Scopes)
	if len(scopes) == 0 {
		scopes = []string{clientScopeContextRW}
	}
	for _, scope := range scopes {
		if !validClientTokenScope(scope) {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "invalid token scope")
			return
		}
	}
	var expiresAt *time.Time
	if strings.TrimSpace(body.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(body.ExpiresAt))
		if err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "expiresAt must be RFC3339")
			return
		}
		expiresAt = &parsed
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	username := cur.Username
	raw := clientTokenPrefix + randomHex(32)
	now := time.Now().UTC()
	rec := clientTokenRecord{
		ID:          "ctok_" + randomHex(8),
		Name:        name,
		TokenHash:   hashClientToken(raw),
		Username:    username,
		Scopes:      scopes,
		CreatedBy:   username,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		Description: strings.TrimSpace(body.Description),
	}
	if err := s.saveClientToken(workspaceID, rec); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	publicRec := rec
	publicRec.TokenHash = ""
	_ = json.NewEncoder(w).Encode(createClientTokenResponse{Token: publicRec, Raw: raw})
}

func (s *Server) handleClientTokensDelete(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "token id is required")
		return
	}
	raw, ok, err := s.controlDB.GetRecord(clientTokensTable, workspaceID, []string{id})
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "client token not found")
		return
	}
	var rec clientTokenRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		s.serverError(w, err)
		return
	}
	if rec.Username != cur.Username {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "client token owner required")
		return
	}
	now := time.Now().UTC()
	rec.RevokedAt = &now
	if err := s.saveClientToken(workspaceID, rec); err != nil {
		s.serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listClientTokens(workspaceID string) ([]clientTokenRecord, error) {
	if s.controlDB == nil {
		return nil, nil
	}
	recs, err := s.controlDB.ListRecords(clientTokensTable, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]clientTokenRecord, 0, len(recs))
	for _, row := range recs {
		var rec clientTokenRecord
		if err := json.Unmarshal([]byte(row.Payload), &rec); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (s *Server) saveClientToken(workspaceID string, rec clientTokenRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return s.controlDB.UpsertRecord(clientTokensTable, workspaceID, []string{rec.ID}, string(raw))
}

func (s *Server) findClientTokenByRaw(workspaceID, rawToken string) (*clientTokenRecord, error) {
	if s.controlDB == nil {
		return nil, nil
	}
	want := hashClientToken(rawToken)
	tokens, err := s.listClientTokens(workspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, rec := range tokens {
		if rec.TokenHash != want {
			continue
		}
		if rec.RevokedAt != nil || (rec.ExpiresAt != nil && rec.ExpiresAt.Before(now)) {
			return nil, nil
		}
		return &rec, nil
	}
	return nil, nil
}

func (s *Server) markClientTokenUsed(workspaceID string, rec clientTokenRecord) {
	if s.controlDB == nil {
		return
	}
	now := time.Now().UTC()
	rec.LastUsedAt = &now
	_ = s.saveClientToken(workspaceID, rec)
}

func hashClientToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeClientTokenScopes(scopes []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func redactClientTokenHashes(tokens []clientTokenRecord) []clientTokenRecord {
	out := make([]clientTokenRecord, len(tokens))
	copy(out, tokens)
	for i := range out {
		out[i].TokenHash = ""
	}
	return out
}

func filterClientTokensForUser(tokens []clientTokenRecord, username string) []clientTokenRecord {
	out := make([]clientTokenRecord, 0, len(tokens))
	for _, token := range tokens {
		if token.Username == username {
			out = append(out, token)
		}
	}
	return out
}

func validClientTokenScope(scope string) bool {
	switch scope {
	case clientScopeContextRW:
		return true
	default:
		return false
	}
}

func (s *Server) requestHasClientScope(r *http.Request, scope string) bool {
	source, _ := r.Context().Value(ctxAuthSourceKey).(identitySource)
	if source != identitySourceClientToken {
		return true
	}
	scopes, _ := r.Context().Value(ctxAuthScopesKey).([]string)
	for _, have := range scopes {
		if have == scope {
			return true
		}
	}
	return false
}

func (s *Server) requireClientScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if s.requestHasClientScope(r, scope) {
		return true
	}
	s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "client token scope required: "+scope)
	return false
}
