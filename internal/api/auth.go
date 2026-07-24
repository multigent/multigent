package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

// System roles.
const (
	RoleAdmin  = string(rbac.OrgRoleAdmin)
	RoleMember = string(rbac.OrgRoleMember)
)

const openRegistrationSettingKey = "auth.open_registration_enabled"

// Project-level roles (ascending privilege).
const (
	ProjectRoleViewer   = string(rbac.ProjectRoleViewer)
	ProjectRoleOperator = string(rbac.ProjectRoleOperator)
	ProjectRoleManager  = string(rbac.ProjectRoleManager)
)

type projectAccess struct {
	Project string `json:"project"`
	Role    string `json:"role"` // viewer | operator | manager
}

type agentAccess struct {
	Project string `json:"project"`
	Agent   string `json:"agent"`
	Role    string `json:"role"` // viewer | operator | owner
}

type invitationRecord struct {
	Token       string          `json:"token"`
	WorkspaceID string          `json:"workspaceId,omitempty"`
	Email       string          `json:"email"`
	Role        string          `json:"role"`
	DisplayName string          `json:"displayName,omitempty"`
	Projects    []projectAccess `json:"projects,omitempty"`
	AgentGrants []agentAccess   `json:"agentGrants,omitempty"`
	InvitedBy   string          `json:"invitedBy,omitempty"`
	Status      string          `json:"status"` // pending | accepted | revoked
	CreatedAt   string          `json:"createdAt"`
	ExpiresAt   string          `json:"expiresAt"`
	AcceptedAt  string          `json:"acceptedAt,omitempty"`
}

type userRecord struct {
	Username     string          `json:"username"`
	Hash         string          `json:"hash"`
	Role         string          `json:"role"` // admin | member
	DisplayName  string          `json:"displayName,omitempty"`
	Email        string          `json:"email,omitempty"`
	Avatar       string          `json:"avatar,omitempty"` // URL
	Phone        string          `json:"phone,omitempty"`
	Bio          string          `json:"bio,omitempty"`
	Projects     []projectAccess `json:"projects,omitempty"`
	AgentGrants  []agentAccess   `json:"agentGrants,omitempty"`
	LinkedAgents []string        `json:"linkedAgents,omitempty"`
	Disabled     bool            `json:"disabled,omitempty"`
	CreatedAt    string          `json:"createdAt,omitempty"`
}

type usersFile struct {
	Users       []userRecord       `json:"users"`
	Invitations []invitationRecord `json:"invitations,omitempty"`
	JWTSecret   string             `json:"jwtSecret"`
}

type UserStore struct {
	db controldb.Store
}

func newUserStore(db controldb.Store) *UserStore {
	s := &UserStore{db: db}
	_ = s.ensureInitialized()
	return s
}

func (s *UserStore) ensureInitialized() error {
	if s.db == nil {
		return fmt.Errorf("control database unavailable")
	}
	if secret, ok, err := s.db.GetSetting("jwt_secret"); err != nil {
		return err
	} else if !ok || secret == "" {
		if err := s.db.SetSetting("jwt_secret", generateSecret()); err != nil {
			return err
		}
	}
	users, err := s.db.ListUsers()
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return nil
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	return s.db.UpsertUser(controldb.User{
		Username:     "admin",
		Role:         RoleAdmin,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

func generateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	email = normalizeEmail(email)
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}

func usernameFromEmail(email string) string {
	local := strings.SplitN(normalizeEmail(email), "@", 2)[0]
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "user"
	}
	return name
}

func (s *UserStore) uniqueUsernameLocked(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "user"
	}
	candidate := base
	for i := 2; ; i++ {
		if _, ok, _ := s.db.UserByUsername(candidate); !ok {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func (s *UserStore) Authenticate(login, password string) *userRecord {
	login = strings.TrimSpace(login)
	u, ok, err := s.db.UserByLogin(login)
	if err != nil || !ok || u.Disabled {
		return nil
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil
	}
	rec := dbUserToRecord(u)
	return &rec
}

func (s *UserStore) userByEmailLocked(email string) *userRecord {
	u, ok, err := s.db.UserByLogin(normalizeEmail(email))
	if err != nil || !ok || normalizeEmail(u.Email) != normalizeEmail(email) {
		return nil
	}
	rec := dbUserToRecord(u)
	return &rec
}

func (s *UserStore) UserByEmail(email string) *userRecord {
	return s.userByEmailLocked(email)
}

func (s *UserStore) ListUsers() []userRecord {
	users, err := s.db.ListUsers()
	if err != nil {
		return nil
	}
	out := make([]userRecord, 0, len(users))
	for _, u := range users {
		out = append(out, dbUserToRecord(u))
	}
	return out
}

func (s *UserStore) CreateUser(username, password, role, displayName, email, avatar, phone, bio string) error {
	email = normalizeEmail(email)
	if _, ok, err := s.db.UserByUsername(username); err != nil {
		return err
	} else if ok {
		return fmt.Errorf("user %q already exists", username)
	}
	if email != "" {
		if u := s.userByEmailLocked(email); u != nil {
			return fmt.Errorf("email %q already exists", email)
		}
	}
	if role != RoleAdmin && role != RoleMember {
		role = RoleMember
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.UpsertUser(recordToDBUser(userRecord{
		Username:    username,
		Hash:        string(hash),
		Role:        role,
		DisplayName: displayName,
		Email:       email,
		Avatar:      avatar,
		Phone:       phone,
		Bio:         bio,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}))
}

func (s *UserStore) RegisterByEmail(email, password, displayName string) (*userRecord, error) {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, fmt.Errorf("valid email required")
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}
	if s.userByEmailLocked(email) != nil {
		return nil, fmt.Errorf("email %q already exists", email)
	}
	username := s.uniqueUsernameLocked(usernameFromEmail(email))
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := userRecord{
		Username:    username,
		Hash:        string(hash),
		Role:        RoleMember,
		DisplayName: displayName,
		Email:       email,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.db.UpsertUser(recordToDBUser(u)); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) CreateInvitation(workspaceID, email, role, displayName, invitedBy string, projects []projectAccess, agentGrants []agentAccess) (*invitationRecord, error) {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return nil, fmt.Errorf("valid email required")
	}
	switch role {
	case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleGuest:
	default:
		role = WorkspaceRoleMember
	}
	projects = normalizeProjectGrants(projects)
	agentGrants = normalizeAgentGrants(agentGrants)
	if err := validateScopedAccessForWorkspaceRole(role, projects, agentGrants); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	inv := invitationRecord{
		Token:       generateToken(24),
		WorkspaceID: workspaceID,
		Email:       email,
		Role:        role,
		DisplayName: displayName,
		Projects:    projects,
		AgentGrants: agentGrants,
		InvitedBy:   invitedBy,
		Status:      "pending",
		CreatedAt:   now.Format(time.RFC3339),
		ExpiresAt:   now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}
	if err := s.db.CreateInvitation(recordToDBInvitation(inv)); err != nil {
		return nil, err
	}
	return &inv, nil
}

func generateToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *UserStore) Invitation(token string) (*invitationRecord, bool) {
	inv, ok, err := s.db.InvitationByToken(token)
	if err != nil || !ok {
		return nil, false
	}
	rec := dbInvitationToRecord(inv)
	return &rec, true
}

func (s *UserStore) ListInvitations(workspaceID string) ([]invitationRecord, error) {
	rows, err := s.db.ListInvitations()
	if err != nil {
		return nil, err
	}
	out := make([]invitationRecord, 0, len(rows))
	for _, row := range rows {
		rec := dbInvitationToRecord(row)
		if workspaceID != "" && rec.WorkspaceID != workspaceID {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func (s *UserStore) SetInvitationStatus(token, status string) (*invitationRecord, error) {
	dbInv, ok, err := s.db.InvitationByToken(token)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invitation not found")
	}
	inv := dbInvitationToRecord(dbInv)
	if inv.Status == "accepted" {
		return nil, fmt.Errorf("accepted invitation cannot be changed")
	}
	switch status {
	case "revoked", "rejected":
		inv.Status = status
	default:
		return nil, fmt.Errorf("unsupported invitation status")
	}
	if err := s.db.UpdateInvitation(recordToDBInvitation(inv)); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *UserStore) AcceptInvitation(token, password, displayName string) (*userRecord, error) {
	now := time.Now().UTC()
	dbInv, ok, err := s.db.InvitationByToken(token)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invitation not found")
	}
	inv := dbInvitationToRecord(dbInv)
	if inv.Status != "pending" {
		return nil, fmt.Errorf("invitation is not active")
	}
	if exp, err := time.Parse(time.RFC3339, inv.ExpiresAt); err == nil && exp.Before(now) {
		inv.Status = "revoked"
		_ = s.db.UpdateInvitation(recordToDBInvitation(inv))
		return nil, fmt.Errorf("invitation expired")
	}
	if existing := s.userByEmailLocked(inv.Email); existing != nil {
		if bcrypt.CompareHashAndPassword([]byte(existing.Hash), []byte(password)) != nil {
			return nil, fmt.Errorf("invalid credentials")
		}
		inv.Status = "accepted"
		inv.AcceptedAt = now.Format(time.RFC3339)
		if err := s.db.UpdateInvitation(recordToDBInvitation(inv)); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}
	if displayName == "" {
		displayName = inv.DisplayName
	}
	username := s.uniqueUsernameLocked(usernameFromEmail(inv.Email))
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := userRecord{
		Username:    username,
		Hash:        string(hash),
		Role:        RoleMember,
		DisplayName: displayName,
		Email:       inv.Email,
		Projects:    inv.Projects,
		AgentGrants: inv.AgentGrants,
		CreatedAt:   now.Format(time.RFC3339),
	}
	if err := s.db.UpsertUser(recordToDBUser(u)); err != nil {
		return nil, err
	}
	inv.Status = "accepted"
	inv.AcceptedAt = now.Format(time.RFC3339)
	if err := s.db.UpdateInvitation(recordToDBInvitation(inv)); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) UpdateUser(username string, role, displayName, email, avatar, phone, bio *string, disabled *bool, projects []projectAccess, agentGrants []agentAccess, newPassword *string) error {
	dbUser, ok, err := s.db.UserByUsername(username)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("user not found")
	}
	u := dbUserToRecord(dbUser)
	if role != nil {
		u.Role = *role
	}
	if displayName != nil {
		u.DisplayName = *displayName
	}
	if email != nil {
		u.Email = *email
	}
	if avatar != nil {
		u.Avatar = *avatar
	}
	if phone != nil {
		u.Phone = *phone
	}
	if bio != nil {
		u.Bio = *bio
	}
	if disabled != nil {
		u.Disabled = *disabled
	}
	if projects != nil {
		u.Projects = projects
	}
	if agentGrants != nil {
		u.AgentGrants = normalizeAgentGrants(agentGrants)
	}
	if newPassword != nil && *newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Hash = string(hash)
	}
	return s.db.UpsertUser(recordToDBUser(u))
}

func (s *UserStore) DeleteUser(username string) error {
	if _, ok, err := s.db.UserByUsername(username); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("user not found")
	}
	return s.db.DeleteUser(username)
}

func (s *UserStore) HasProjectAccess(username, project string) (string, bool) {
	u := s.GetUser(username)
	if u == nil {
		return "", false
	}
	if u.Role == RoleAdmin {
		return ProjectRoleManager, true
	}
	for _, pa := range u.Projects {
		if pa.Project == project {
			return pa.Role, true
		}
	}
	return "", false
}

func (s *UserStore) Principal(username string) (rbac.Principal, bool) {
	u := s.GetUser(username)
	if u == nil || u.Disabled {
		return rbac.Principal{}, false
	}
	p := rbac.Principal{
		ID:           u.Username,
		OrgRole:      rbac.Role(u.Role),
		ProjectRoles: make(map[string]rbac.Role, len(u.Projects)),
		AgentRoles:   make(map[string]rbac.Role, len(u.AgentGrants)),
		TaskRoles:    make(map[string]rbac.Role),
		ContextRoles: make(map[string]rbac.Role),
		WorkerRoles:  make(map[string]rbac.Role),
	}
	for _, pa := range u.Projects {
		p.ProjectRoles[rbac.ProjectKey(pa.Project)] = rbac.Role(pa.Role)
	}
	for _, grant := range u.AgentGrants {
		if grant.Project != "" && grant.Agent != "" {
			p.AgentRoles[rbac.AgentKey(grant.Project, grant.Agent)] = rbac.Role(grant.Role)
		}
	}
	return p, true
}

func projectRoleLevel(role string) int {
	return rbac.ProjectRolePower(rbac.Role(role))
}

func agentRoleLevel(role string) int {
	return rbac.RolePower(rbac.ResourceAgent, rbac.Role(role))
}

func normalizeAgentGrants(in []agentAccess) []agentAccess {
	out := make([]agentAccess, 0, len(in))
	seen := map[string]bool{}
	for _, grant := range in {
		project := strings.TrimSpace(grant.Project)
		agent := strings.TrimSpace(grant.Agent)
		role := strings.TrimSpace(grant.Role)
		if project == "" || agent == "" {
			continue
		}
		switch role {
		case string(rbac.AgentRoleOwner), string(rbac.AgentRoleOperator), string(rbac.AgentRoleViewer):
		default:
			role = string(rbac.AgentRoleOperator)
		}
		key := project + "/" + agent
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, agentAccess{Project: project, Agent: agent, Role: role})
	}
	return out
}

func normalizeProjectGrants(in []projectAccess) []projectAccess {
	out := make([]projectAccess, 0, len(in))
	seen := map[string]bool{}
	for _, grant := range in {
		project := strings.TrimSpace(grant.Project)
		role := strings.TrimSpace(grant.Role)
		if project == "" {
			continue
		}
		switch role {
		case ProjectRoleViewer, ProjectRoleOperator, ProjectRoleManager:
		default:
			role = ProjectRoleViewer
		}
		if seen[project] {
			continue
		}
		seen[project] = true
		out = append(out, projectAccess{Project: project, Role: role})
	}
	return out
}

func validateScopedAccessForWorkspaceRole(workspaceRole string, projects []projectAccess, agentGrants []agentAccess) error {
	if workspaceRole != WorkspaceRoleGuest {
		return nil
	}
	for _, grant := range normalizeProjectGrants(projects) {
		if grant.Role != ProjectRoleViewer {
			return fmt.Errorf("workspace guest can only receive project viewer access")
		}
	}
	if len(normalizeAgentGrants(agentGrants)) > 0 {
		return fmt.Errorf("workspace guest cannot receive agent access")
	}
	return nil
}

func downgradeScopedAccessForWorkspaceGuest(projects []projectAccess) []projectAccess {
	normalized := normalizeProjectGrants(projects)
	for i := range normalized {
		normalized[i].Role = ProjectRoleViewer
	}
	return normalized
}

func linkedAgentRefs(grants []agentAccess) []string {
	out := make([]string, 0, len(grants))
	for _, grant := range normalizeAgentGrants(grants) {
		out = append(out, grant.Project+"/"+grant.Agent)
	}
	return out
}

func dbUserToRecord(u controldb.User) userRecord {
	var projects []projectAccess
	var grants []agentAccess
	_ = json.Unmarshal([]byte(u.ProjectsJSON), &projects)
	_ = json.Unmarshal([]byte(u.LinkedJSON), &grants)
	return userRecord{
		Username:     u.Username,
		Hash:         u.PasswordHash,
		Role:         u.Role,
		DisplayName:  u.DisplayName,
		Email:        u.Email,
		Avatar:       u.Avatar,
		Phone:        u.Phone,
		Bio:          u.Bio,
		Projects:     normalizeProjectGrants(projects),
		AgentGrants:  normalizeAgentGrants(grants),
		LinkedAgents: linkedAgentRefs(grants),
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt,
	}
}

func recordToDBUser(u userRecord) controldb.User {
	projects, _ := json.Marshal(normalizeProjectGrants(u.Projects))
	grants := normalizeAgentGrants(u.AgentGrants)
	linked, _ := json.Marshal(grants)
	return controldb.User{
		Username:     u.Username,
		Email:        normalizeEmail(u.Email),
		DisplayName:  u.DisplayName,
		Role:         u.Role,
		Avatar:       u.Avatar,
		Phone:        u.Phone,
		Bio:          u.Bio,
		PasswordHash: u.Hash,
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt,
		ProjectsJSON: string(projects),
		LinkedJSON:   string(linked),
	}
}

func dbInvitationToRecord(inv controldb.Invitation) invitationRecord {
	var projects []projectAccess
	var grants []agentAccess
	_ = json.Unmarshal([]byte(inv.ProjectsJSON), &projects)
	_ = json.Unmarshal([]byte(inv.LinkedJSON), &grants)
	return invitationRecord{
		Token:       inv.Token,
		WorkspaceID: inv.WorkspaceID,
		Email:       inv.Email,
		Role:        inv.Role,
		DisplayName: inv.DisplayName,
		Projects:    normalizeProjectGrants(projects),
		AgentGrants: normalizeAgentGrants(grants),
		InvitedBy:   inv.InvitedBy,
		Status:      inv.Status,
		CreatedAt:   inv.CreatedAt,
		ExpiresAt:   inv.ExpiresAt,
		AcceptedAt:  inv.AcceptedAt,
	}
}

func recordToDBInvitation(inv invitationRecord) controldb.Invitation {
	projects, _ := json.Marshal(normalizeProjectGrants(inv.Projects))
	linked, _ := json.Marshal(normalizeAgentGrants(inv.AgentGrants))
	return controldb.Invitation{
		Token:        inv.Token,
		WorkspaceID:  inv.WorkspaceID,
		Email:        normalizeEmail(inv.Email),
		Role:         inv.Role,
		DisplayName:  inv.DisplayName,
		ProjectsJSON: string(projects),
		LinkedJSON:   string(linked),
		InvitedBy:    inv.InvitedBy,
		Status:       inv.Status,
		CreatedAt:    inv.CreatedAt,
		ExpiresAt:    inv.ExpiresAt,
		AcceptedAt:   inv.AcceptedAt,
	}
}

func (s *UserStore) ChangePassword(username, oldPass, newPass string) error {
	u := s.GetUser(username)
	if u == nil {
		return fmt.Errorf("user not found")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Hash), []byte(oldPass)) != nil {
		return fmt.Errorf("wrong old password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Hash = string(hash)
	return s.db.UpsertUser(recordToDBUser(*u))
}

func (s *UserStore) GetUser(username string) *userRecord {
	u, ok, err := s.db.UserByUsername(username)
	if err != nil || !ok {
		return nil
	}
	rec := dbUserToRecord(u)
	return &rec
}

func (s *UserStore) Secret() string {
	secret, ok, err := s.db.GetSetting("jwt_secret")
	if err == nil && ok && secret != "" {
		return secret
	}
	secret = generateSecret()
	_ = s.db.SetSetting("jwt_secret", secret)
	return secret
}

// Simple JWT: header.payload.signature with HMAC-SHA256.

type jwtPayload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

func (s *UserStore) IssueToken(username string, dur time.Duration) string {
	now := time.Now()
	payload := jwtPayload{Sub: username, Exp: now.Add(dur).Unix(), Iat: now.Unix()}
	return s.signJWT(payload)
}

func (s *UserStore) ValidateToken(token string) (string, bool) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", false
	}
	wantSig := s.hmacSign(parts[0] + "." + parts[1])
	if parts[2] != wantSig {
		return "", false
	}
	raw, err := base64Decode(parts[1])
	if err != nil {
		return "", false
	}
	var p jwtPayload
	if json.Unmarshal(raw, &p) != nil {
		return "", false
	}
	if time.Now().Unix() > p.Exp {
		return "", false
	}
	return p.Sub, true
}

func (s *UserStore) signJWT(p jwtPayload) string {
	header := base64Encode([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(p)
	payloadB64 := base64Encode(payload)
	sig := s.hmacSign(header + "." + payloadB64)
	return header + "." + payloadB64 + "." + sig
}

func (s *UserStore) hmacSign(msg string) string {
	mac := hmac.New(sha256.New, []byte(s.Secret()))
	mac.Write([]byte(msg))
	return base64Encode(mac.Sum(nil))
}

func base64Encode(data []byte) string {
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	result := make([]byte, 0, (len(data)*4+2)/3)
	for i := 0; i < len(data); i += 3 {
		val := uint(data[i]) << 16
		if i+1 < len(data) {
			val |= uint(data[i+1]) << 8
		}
		if i+2 < len(data) {
			val |= uint(data[i+2])
		}
		result = append(result, enc[(val>>18)&0x3F])
		result = append(result, enc[(val>>12)&0x3F])
		if i+1 < len(data) {
			result = append(result, enc[(val>>6)&0x3F])
		}
		if i+2 < len(data) {
			result = append(result, enc[val&0x3F])
		}
	}
	return string(result)
}

func base64Decode(s string) ([]byte, error) {
	const dec = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var lookup [256]byte
	for i := range lookup {
		lookup[i] = 0xFF
	}
	for i, c := range dec {
		lookup[c] = byte(i)
	}

	out := make([]byte, 0, len(s)*3/4)
	buf := make([]byte, 0, 4)
	for i := 0; i < len(s); i++ {
		v := lookup[s[i]]
		if v == 0xFF {
			continue
		}
		buf = append(buf, v)
		if len(buf) == 4 {
			out = append(out, byte(buf[0]<<2|buf[1]>>4))
			out = append(out, byte(buf[1]<<4|buf[2]>>2))
			out = append(out, byte(buf[2]<<6|buf[3]))
			buf = buf[:0]
		}
	}
	switch len(buf) {
	case 3:
		out = append(out, byte(buf[0]<<2|buf[1]>>4))
		out = append(out, byte(buf[1]<<4|buf[2]>>2))
	case 2:
		out = append(out, byte(buf[0]<<2|buf[1]>>4))
	}
	return out, nil
}

// HTTP handlers

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" {
		body.Username = strings.TrimSpace(body.Email)
	}
	body.Password = strings.TrimSpace(body.Password)
	if body.Username == "" || body.Password == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "username and password required")
		return
	}

	user := s.users.Authenticate(body.Username, body.Password)
	if user == nil {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeInvalidCredentials, "invalid credentials")
		return
	}

	s.issueLoginResponse(w, user)
}

func (s *Server) issueLoginResponse(w http.ResponseWriter, user *userRecord) {
	token := s.users.IssueToken(user.Username, 7*24*time.Hour)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":        token,
		"username":     user.Username,
		"role":         user.Role,
		"displayName":  user.DisplayName,
		"email":        user.Email,
		"avatar":       user.Avatar,
		"projects":     user.Projects,
		"agentGrants":  user.AgentGrants,
		"linkedAgents": user.LinkedAgents,
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.openRegistrationEnabled() {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeSignupDisabled, "signup is disabled")
		return
	}
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	user, err := s.users.RegisterByEmail(body.Email, strings.TrimSpace(body.Password), strings.TrimSpace(body.DisplayName))
	if err != nil {
		if strings.Contains(err.Error(), "exists") {
			s.jsonErrorCode(w, http.StatusConflict, ErrCodeUserAlreadyExists, err.Error())
			return
		}
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.issueLoginResponse(w, user)
}

func (s *Server) openRegistrationEnabled() bool {
	if strings.EqualFold(os.Getenv("MULTIGENT_ALLOW_SIGNUP"), "false") {
		return false
	}
	if s.controlDB == nil {
		return true
	}
	raw, ok, err := s.controlDB.GetSetting(openRegistrationSettingKey)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
	}
}

func (s *Server) handlePublicAuthSettings(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"openRegistrationEnabled": s.openRegistrationEnabled(),
	})
}

func (s *Server) handleAuthSettings(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"openRegistrationEnabled": s.openRegistrationEnabled(),
		"envSignupDisabled":       strings.EqualFold(os.Getenv("MULTIGENT_ALLOW_SIGNUP"), "false"),
	})
}

func (s *Server) handlePutAuthSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		OpenRegistrationEnabled bool `json:"openRegistrationEnabled"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	value := "false"
	if body.OpenRegistrationEnabled {
		value = "true"
	}
	if err := s.controlDB.SetSetting(openRegistrationSettingKey, value); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  "system",
		Action:       "auth.settings.update",
		ResourceType: "auth_settings",
		ResourceID:   openRegistrationSettingKey,
		Summary:      "Authentication settings updated",
		After:        map[string]any{"openRegistrationEnabled": body.OpenRegistrationEnabled},
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{
		"openRegistrationEnabled": s.openRegistrationEnabled(),
		"envSignupDisabled":       strings.EqualFold(os.Getenv("MULTIGENT_ALLOW_SIGNUP"), "false"),
	})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ctxUserKey).(string)
	user := s.users.GetUser(username)
	if user == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, "user not found")
		return
	}
	workspaceRole := ""
	currentUserCanAdmin := false
	if workspaceID, err := s.ensureCurrentWorkspaceForUser(r); err == nil && workspaceID != "" {
		workspaceRole, currentUserCanAdmin = s.currentWorkspaceRole(r, workspaceID)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"username":            user.Username,
		"role":                user.Role,
		"workspaceRole":       workspaceRole,
		"currentUserCanAdmin": currentUserCanAdmin,
		"displayName":         user.DisplayName,
		"email":               user.Email,
		"avatar":              user.Avatar,
		"projects":            user.Projects,
		"agentGrants":         user.AgentGrants,
		"linkedAgents":        user.LinkedAgents,
	})
}

// RBAC helpers

func (s *Server) currentUser(r *http.Request) *userRecord {
	username, _ := r.Context().Value(ctxUserKey).(string)
	if username == "" || username == "apikey" {
		return &userRecord{Username: username, Role: RoleAdmin}
	}
	u := s.users.GetUser(username)
	if u == nil {
		return &userRecord{Username: username, Role: RoleMember}
	}
	return u
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	u := s.currentUser(r)
	if u.Role != RoleAdmin {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAdminRequired, "admin access required")
		return false
	}
	return true
}

func (s *Server) canAccessProject(r *http.Request, project string) bool {
	if s.canAccessWholeProject(r, project) {
		return true
	}
	return currentUserLinkedProject(s.currentUser(r), project)
}

func (s *Server) canAccessWholeProject(r *http.Request, project string) bool {
	u := s.currentUser(r)
	if u.Role == RoleAdmin || s.canAdminCurrentWorkspace(r) {
		return true
	}
	_, ok := s.users.HasProjectAccess(u.Username, project)
	return ok
}

func currentUserLinkedProject(cur *userRecord, project string) bool {
	if cur == nil {
		return false
	}
	for _, grant := range cur.AgentGrants {
		if grant.Project == project {
			return true
		}
	}
	return false
}

func (s *Server) projectRole(r *http.Request, project string) (string, bool) {
	u := s.currentUser(r)
	if u.Role == RoleAdmin || s.canAdminCurrentWorkspace(r) {
		return ProjectRoleManager, true
	}
	return s.users.HasProjectAccess(u.Username, project)
}

func (s *Server) canOperateProject(r *http.Request, project string) bool {
	role, ok := s.projectRole(r, project)
	return ok && projectRoleLevel(role) >= projectRoleLevel(ProjectRoleOperator)
}

func (s *Server) canManageProject(r *http.Request, project string) bool {
	role, ok := s.projectRole(r, project)
	return ok && projectRoleLevel(role) >= projectRoleLevel(ProjectRoleManager)
}

func (s *Server) checkProjectOperator(w http.ResponseWriter, r *http.Request, project string) bool {
	if !s.canOperateProject(r, project) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectOperatorRequired, "project operator access required")
		return false
	}
	return true
}

func (s *Server) canAccessAgent(r *http.Request, project, agent string) bool {
	if s.canAccessWholeProject(r, project) {
		return true
	}
	_, ok := currentUserAgentRole(s.currentUser(r), project, agent)
	return ok
}

func (s *Server) checkAgentAccess(w http.ResponseWriter, r *http.Request, project, agent string) bool {
	if !s.canAccessAgent(r, project, agent) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAgentAccessRequired, "agent access required")
		return false
	}
	return true
}

func (s *Server) checkProjectManager(w http.ResponseWriter, r *http.Request, project string) bool {
	if !s.canManageProject(r, project) {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeProjectManagerRequired, "project manager access required")
		return false
	}
	return true
}

// User management handlers

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.ensureCurrentWorkspaceForUser(r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}
	members, err := s.controlDB.ListWorkspaceMembers(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAccessRequired, "workspace access required")
		return
	}
	isMember := cur.Role == RoleAdmin || cur.Username == "apikey"
	for _, member := range members {
		if member.Username == cur.Username {
			isMember = true
			break
		}
	}
	if !isMember {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAccessRequired, "workspace access required")
		return
	}
	isAdmin := cur.Role == RoleAdmin || s.canAdminWorkspace(r, workspaceID)
	type safeUser struct {
		Username     string          `json:"username"`
		Role         string          `json:"role"`
		SystemRole   string          `json:"systemRole,omitempty"`
		DisplayName  string          `json:"displayName,omitempty"`
		Email        string          `json:"email,omitempty"`
		Avatar       string          `json:"avatar,omitempty"`
		Phone        string          `json:"phone,omitempty"`
		Bio          string          `json:"bio,omitempty"`
		Projects     []projectAccess `json:"projects,omitempty"`
		AgentGrants  []agentAccess   `json:"agentGrants,omitempty"`
		LinkedAgents []string        `json:"linkedAgents,omitempty"`
		Disabled     bool            `json:"disabled,omitempty"`
		CreatedAt    string          `json:"createdAt,omitempty"`
	}
	out := make([]safeUser, 0, len(members))
	for _, member := range members {
		u := s.users.GetUser(member.Username)
		if u == nil {
			continue
		}
		row := safeUser{
			Username:    u.Username,
			Role:        member.Role,
			SystemRole:  u.Role,
			DisplayName: u.DisplayName,
			Email:       u.Email,
			Avatar:      u.Avatar,
			CreatedAt:   member.CreatedAt,
		}
		if isAdmin || cur.Username == u.Username {
			row.Phone = u.Phone
			row.Bio = u.Bio
			row.Projects = u.Projects
			row.AgentGrants = u.AgentGrants
			row.LinkedAgents = u.LinkedAgents
			row.Disabled = u.Disabled
		}
		out = append(out, row)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleLookupUserByEmail(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	email := normalizeEmail(r.URL.Query().Get("email"))
	if !validEmail(email) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "valid email required")
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var user *userRecord
	if u := s.users.UserByEmail(email); u != nil {
		user = u
	}
	alreadyMember := false
	if user != nil && s.controlDB != nil {
		if _, ok, err := s.controlDB.WorkspaceMember(workspaceID, user.Username); err != nil {
			s.serverError(w, err)
			return
		} else {
			alreadyMember = ok
		}
	}
	pendingInvite := false
	invitations, err := s.users.ListInvitations(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	for _, inv := range invitations {
		if normalizeEmail(inv.Email) == email && inv.Status == "pending" {
			pendingInvite = true
			break
		}
	}
	resp := map[string]any{
		"email":         email,
		"registered":    user != nil,
		"alreadyMember": alreadyMember,
		"pendingInvite": pendingInvite,
	}
	if user != nil {
		resp["user"] = map[string]any{
			"username":    user.Username,
			"displayName": user.DisplayName,
			"email":       user.Email,
			"avatar":      user.Avatar,
			"disabled":    user.Disabled,
		}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		Role          string `json:"role"`
		WorkspaceRole string `json:"workspaceRole"`
		DisplayName   string `json:"displayName"`
		Email         string `json:"email"`
		Avatar        string `json:"avatar"`
		Phone         string `json:"phone"`
		Bio           string `json:"bio"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	body.Email = normalizeEmail(body.Email)
	body.Password = strings.TrimSpace(body.Password)
	if body.Username == "" && body.Email != "" {
		body.Username = s.users.uniqueUsernameLocked(usernameFromEmail(body.Email))
	}
	if body.Username == "" || body.Password == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "email and password required")
		return
	}
	if body.Email != "" && !validEmail(body.Email) {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "valid email required")
		return
	}
	if len(body.Password) < 6 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "password must be at least 6 characters")
		return
	}
	workspaceRole := WorkspaceRoleMember
	workspaceID, workspaceErr := s.currentWorkspaceID()
	if workspaceErr == nil && workspaceID != "" {
		switch body.WorkspaceRole {
		case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleGuest:
			workspaceRole = body.WorkspaceRole
		case "":
			if body.Role == WorkspaceRoleAdmin || body.Role == RoleAdmin {
				workspaceRole = WorkspaceRoleAdmin
			}
		default:
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "invalid workspace role")
			return
		}
		if body.WorkspaceRole == WorkspaceRoleOwner {
			cur := s.currentUser(r)
			currentRole, _ := s.currentWorkspaceRole(r, workspaceID)
			if currentRole != WorkspaceRoleOwner && (cur == nil || cur.Role != RoleAdmin) {
				s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAdminRequired, "workspace owner access required")
				return
			}
		}
	}
	if err := s.users.CreateUser(body.Username, body.Password, body.Role, body.DisplayName, body.Email, body.Avatar, body.Phone, body.Bio); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			s.jsonErrorCode(w, http.StatusConflict, ErrCodeUserAlreadyExists, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	if workspaceErr == nil && workspaceID != "" {
		if err := s.controlDB.UpsertWorkspaceMember(workspaceID, body.Username, workspaceRole); err != nil {
			s.serverError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) invitationURL(r *http.Request, token string) string {
	if origin := strings.TrimRight(r.Header.Get("Origin"), "/"); origin != "" {
		return fmt.Sprintf("%s/invite/%s", origin, token)
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	return fmt.Sprintf("%s://%s/invite/%s", proto, host, token)
}

func (s *Server) handleCreateInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body struct {
		Email       string          `json:"email"`
		Emails      []string        `json:"emails"`
		Role        string          `json:"role"`
		DisplayName string          `json:"displayName"`
		Projects    []projectAccess `json:"projects"`
		AgentGrants []agentAccess   `json:"agentGrants"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	cur := s.currentUser(r)
	emails := invitationEmails(body.Email, body.Emails)
	if len(emails) == 0 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "email is required")
		return
	}
	role, ok := s.allowedInviteRole(r, body.Role)
	if !ok {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "workspace owner access required for this role")
		return
	}
	body.Projects = normalizeProjectGrants(body.Projects)
	body.AgentGrants = normalizeAgentGrants(body.AgentGrants)
	if err := validateScopedAccessForWorkspaceRole(role, body.Projects, body.AgentGrants); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}

	type invitationResult struct {
		Invitation *invitationRecord `json:"invitation"`
		InviteURL  string            `json:"inviteUrl"`
		Delivery   string            `json:"delivery,omitempty"`
	}
	type invitationError struct {
		Email string `json:"email"`
		Error string `json:"error"`
	}
	results := make([]invitationResult, 0, len(emails))
	errors := make([]invitationError, 0)
	smtpCfg, smtpEnabled := loadSMTPConfig()
	delivery := "local-link"
	if smtpEnabled {
		delivery = "smtp"
	}
	workspaceName := "Multigent workspace"
	if ws, ok, err := s.controlDB.WorkspaceByID(workspaceID); err == nil && ok && strings.TrimSpace(ws.Name) != "" {
		workspaceName = strings.TrimSpace(ws.Name)
	}
	inviterName := strings.TrimSpace(cur.DisplayName)
	if inviterName == "" {
		inviterName = strings.TrimSpace(cur.Email)
	}
	if inviterName == "" {
		inviterName = strings.TrimSpace(cur.Username)
	}
	for _, email := range emails {
		inv, err := s.users.CreateInvitation(workspaceID, email, role, strings.TrimSpace(body.DisplayName), cur.Username, body.Projects, body.AgentGrants)
		if err != nil {
			errors = append(errors, invitationError{Email: email, Error: err.Error()})
			continue
		}
		inviteURL := s.invitationURL(r, inv.Token)
		result := invitationResult{Invitation: inv, InviteURL: inviteURL, Delivery: delivery}
		if smtpEnabled {
			if err := smtpCfg.sendInvitation(invitationEmailData{
				To:            inv.Email,
				DisplayName:   inv.DisplayName,
				InviteURL:     inviteURL,
				WorkspaceName: workspaceName,
				InviterName:   inviterName,
				ExpiresAt:     inv.ExpiresAt,
				Locale:        r.Header.Get("Accept-Language"),
			}); err != nil {
				result.Delivery = "smtp_failed"
				errors = append(errors, invitationError{Email: email, Error: "invitation created, but email delivery failed: " + err.Error()})
			}
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "delivery": "local-link", "invitations": results, "errors": errors})
		return
	}
	response := map[string]any{
		"ok":          true,
		"invitations": results,
		"errors":      errors,
		"delivery":    delivery,
	}
	if len(results) == 1 {
		response["invitation"] = results[0].Invitation
		response["inviteUrl"] = results[0].InviteURL
	}
	_ = json.NewEncoder(w).Encode(response)
}

func invitationEmails(email string, emails []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(emails)+1)
	add := func(value string) {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			switch r {
			case ',', ';', '\n', '\r', '\t', ' ':
				return true
			default:
				return false
			}
		}) {
			normalized := strings.ToLower(strings.TrimSpace(part))
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	add(email)
	for _, value := range emails {
		add(value)
	}
	return out
}

func (s *Server) allowedInviteRole(r *http.Request, requested string) (string, bool) {
	switch requested {
	case WorkspaceRoleOwner:
	case WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleGuest:
		return requested, true
	default:
		return WorkspaceRoleMember, true
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return "", false
	}
	role, _ := s.currentWorkspaceRole(r, workspaceID)
	return WorkspaceRoleOwner, role == WorkspaceRoleOwner
}

func (s *Server) handleListInvitations(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	invitations, err := s.users.ListInvitations(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"invitations": invitations})
}

func (s *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if inv, ok := s.users.Invitation(r.PathValue("token")); ok && inv.WorkspaceID != workspaceID {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeInvitationNotFound, "invitation not found")
		return
	}
	inv, err := s.users.SetInvitationStatus(r.PathValue("token"), "revoked")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeInvitationNotFound, err.Error())
			return
		}
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "invitation": inv})
}

func (s *Server) handlePublicInvitation(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	inv, ok := s.users.Invitation(token)
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeInvitationNotFound, "invitation not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"email":       inv.Email,
		"role":        inv.Role,
		"displayName": inv.DisplayName,
		"status":      inv.Status,
		"expiresAt":   inv.ExpiresAt,
	})
}

func (s *Server) handleRejectInvitation(w http.ResponseWriter, r *http.Request) {
	inv, err := s.users.SetInvitationStatus(r.PathValue("token"), "rejected")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeInvitationNotFound, err.Error())
			return
		}
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "invitation": inv})
}

func (s *Server) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	var body struct {
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	user, err := s.users.AcceptInvitation(token, strings.TrimSpace(body.Password), strings.TrimSpace(body.DisplayName))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeInvitationNotFound, err.Error())
			return
		}
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if inv, ok := s.users.Invitation(token); ok && inv.WorkspaceID != "" && s.controlDB != nil {
		if err := s.controlDB.UpsertWorkspaceMember(inv.WorkspaceID, user.Username, invitationWorkspaceRole(user, token, s.users)); err != nil {
			s.serverError(w, err)
			return
		}
	}
	s.issueLoginResponse(w, user)
}

func invitationWorkspaceRole(user *userRecord, token string, users *UserStore) string {
	if users != nil {
		if inv, ok := users.Invitation(token); ok {
			switch inv.Role {
			case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleGuest:
				return inv.Role
			}
		}
	}
	if user != nil && user.Role == RoleAdmin {
		return WorkspaceRoleAdmin
	}
	return WorkspaceRoleMember
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("username")
	cur := s.currentUser(r)
	if cur.Role != RoleAdmin && cur.Username != target {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "access denied")
		return
	}
	u := s.users.GetUser(target)
	if u == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, "user not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"username":     u.Username,
		"role":         u.Role,
		"displayName":  u.DisplayName,
		"email":        u.Email,
		"avatar":       u.Avatar,
		"phone":        u.Phone,
		"bio":          u.Bio,
		"projects":     u.Projects,
		"agentGrants":  u.AgentGrants,
		"linkedAgents": u.LinkedAgents,
		"disabled":     u.Disabled,
		"createdAt":    u.CreatedAt,
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("username")
	cur := s.currentUser(r)
	workspaceID, _ := s.currentWorkspaceID()
	isSystemAdmin := cur.Role == RoleAdmin
	isWorkspaceAdmin := workspaceID != "" && s.canAdminWorkspace(r, workspaceID)
	isSelf := cur.Username == target
	if !isSystemAdmin && !isWorkspaceAdmin && !isSelf {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "access denied")
		return
	}
	targetUser := s.users.GetUser(target)
	if targetUser == nil {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, "user not found")
		return
	}
	if isWorkspaceAdmin && !isSystemAdmin && !isSelf {
		if s.controlDB == nil || workspaceID == "" {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAccessRequired, "workspace access required")
			return
		}
		if _, ok, err := s.controlDB.WorkspaceMember(workspaceID, target); err != nil {
			s.serverError(w, err)
			return
		} else if !ok {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeWorkspaceAccessRequired, "target user is not a workspace member")
			return
		}
		if targetUser.Role == RoleAdmin {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "super admin cannot be modified from workspace permissions")
			return
		}
	}
	var body struct {
		Role        *string         `json:"role"`
		DisplayName *string         `json:"displayName"`
		Email       *string         `json:"email"`
		Avatar      *string         `json:"avatar"`
		Phone       *string         `json:"phone"`
		Bio         *string         `json:"bio"`
		Disabled    *bool           `json:"disabled"`
		Password    *string         `json:"password"`
		Projects    []projectAccess `json:"projects"`
		AgentGrants []agentAccess   `json:"agentGrants"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	if body.Password != nil && len(*body.Password) > 0 && len(*body.Password) < 6 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "password must be at least 6 characters")
		return
	}
	if !isSystemAdmin {
		body.Role = nil
		body.Email = nil
		body.Phone = nil
		body.Bio = nil
		body.Disabled = nil
		body.Password = nil
	}
	if !isSystemAdmin && !isWorkspaceAdmin {
		body.Projects = nil
		body.AgentGrants = nil
	}
	targetWorkspaceRole := WorkspaceRoleMember
	if targetUser.Role == RoleAdmin {
		targetWorkspaceRole = WorkspaceRoleAdmin
	} else if workspaceID != "" && s.controlDB != nil {
		if member, ok, err := s.controlDB.WorkspaceMember(workspaceID, target); err != nil {
			s.serverError(w, err)
			return
		} else if ok {
			targetWorkspaceRole = member.Role
		}
	}
	if body.Projects != nil {
		body.Projects = normalizeProjectGrants(body.Projects)
	}
	if body.AgentGrants != nil {
		body.AgentGrants = normalizeAgentGrants(body.AgentGrants)
	}
	if err := validateScopedAccessForWorkspaceRole(targetWorkspaceRole, body.Projects, body.AgentGrants); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if err := s.users.UpdateUser(target, body.Role, body.DisplayName, body.Email, body.Avatar, body.Phone, body.Bio, body.Disabled, body.Projects, body.AgentGrants, body.Password); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	u := s.users.GetUser(target)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":           true,
		"username":     u.Username,
		"role":         u.Role,
		"displayName":  u.DisplayName,
		"email":        u.Email,
		"avatar":       u.Avatar,
		"projects":     u.Projects,
		"agentGrants":  u.AgentGrants,
		"linkedAgents": u.LinkedAgents,
	})
}

func (s *Server) handleUpdateWorkspaceMemberRole(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	target := strings.TrimSpace(r.PathValue("username"))
	if target == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "username required")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	nextRole := strings.TrimSpace(body.Role)
	switch nextRole {
	case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleMember, WorkspaceRoleGuest:
	default:
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "unsupported workspace role")
		return
	}
	currentRole, _ := s.currentWorkspaceRole(r, workspaceID)
	member, ok, err := s.controlDB.WorkspaceMember(workspaceID, target)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, "workspace member not found")
		return
	}
	if currentRole != WorkspaceRoleOwner {
		if nextRole == WorkspaceRoleOwner || member.Role == WorkspaceRoleOwner {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeForbidden, "workspace owner access required")
			return
		}
	}
	if member.Role == WorkspaceRoleOwner && nextRole != WorkspaceRoleOwner {
		members, err := s.controlDB.ListWorkspaceMembers(workspaceID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		ownerCount := 0
		for _, m := range members {
			if m.Role == WorkspaceRoleOwner {
				ownerCount++
			}
		}
		if ownerCount <= 1 {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "workspace must keep at least one owner")
			return
		}
	}
	if err := s.controlDB.UpsertWorkspaceMember(workspaceID, target, nextRole); err != nil {
		s.serverError(w, err)
		return
	}
	if nextRole == WorkspaceRoleGuest {
		if targetUser := s.users.GetUser(target); targetUser != nil {
			projects := downgradeScopedAccessForWorkspaceGuest(targetUser.Projects)
			if err := s.users.UpdateUser(target, nil, nil, nil, nil, nil, nil, nil, projects, []agentAccess{}, nil); err != nil {
				s.serverError(w, err)
				return
			}
		}
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "workspace_member.role.update",
		ResourceType: "workspace_member",
		ResourceID:   target,
		Summary:      "Workspace member role updated",
		Before:       map[string]any{"role": member.Role},
		After:        map[string]any{"role": nextRole},
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "username": target, "role": nextRole})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	target := r.PathValue("username")
	cur := s.currentUser(r)
	if target == cur.Username {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "cannot delete yourself")
		return
	}
	if err := s.users.DeleteUser(target); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonErrorCode(w, http.StatusNotFound, ErrCodeUserNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	username := r.Context().Value(ctxUserKey).(string)
	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidRequestBody, "invalid request body")
		return
	}
	if len(body.NewPassword) < 6 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "password must be at least 6 characters")
		return
	}
	if err := s.users.ChangePassword(username, body.OldPassword, body.NewPassword); err != nil {
		if strings.Contains(err.Error(), "wrong old password") {
			s.jsonErrorCode(w, http.StatusForbidden, ErrCodeInvalidCredentials, "wrong old password")
			return
		}
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
