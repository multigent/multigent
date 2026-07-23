package store

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/secretbox"
)

type ProviderStore struct {
	root string
	db   controldb.Store
}

func NewProviderStore(root string) *ProviderStore {
	return &ProviderStore{root: root}
}

func NewProviderStoreWithDB(root string, db controldb.Store) *ProviderStore {
	return &ProviderStore{root: root, db: db}
}

func newProviderID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[r.Intn(len(chars))]
	}
	return "prov-" + string(b)
}

func (ps *ProviderStore) List() ([]entity.APIProvider, error) {
	db, workspaceID, cleanup, err := ps.openWorkspaceDB()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	rows, err := db.ListModelProviders(workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]entity.APIProvider, 0, len(rows))
	for _, row := range rows {
		provider, err := modelProviderFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, provider)
	}
	return out, nil
}

func (ps *ProviderStore) Get(id string) (*entity.APIProvider, error) {
	db, workspaceID, cleanup, err := ps.openWorkspaceDB()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	row, ok, err := db.ModelProviderByID(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider %q not found", id)
	}
	provider, err := modelProviderFromDB(row)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

func (ps *ProviderStore) Add(p entity.APIProvider) (*entity.APIProvider, error) {
	db, workspaceID, cleanup, err := ps.openWorkspaceDB()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	p.ID = newProviderID()
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, fmt.Errorf("provider name is required")
	}
	row, err := modelProviderToDB(workspaceID, p)
	if err != nil {
		return nil, err
	}
	if err := db.UpsertModelProvider(workspaceID, row); err != nil {
		return nil, err
	}
	return &p, nil
}

func (ps *ProviderStore) Update(id string, p entity.APIProvider) (*entity.APIProvider, error) {
	db, workspaceID, cleanup, err := ps.openWorkspaceDB()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	existing, ok, err := db.ModelProviderByID(workspaceID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("provider %q not found", id)
	}
	p.ID = id
	p.OwnerType = existing.OwnerType
	p.OwnerID = existing.OwnerID
	row, err := modelProviderToDB(workspaceID, p)
	if err != nil {
		return nil, err
	}
	row.CreatedAt = existing.CreatedAt
	if err := db.UpsertModelProvider(workspaceID, row); err != nil {
		return nil, err
	}
	return &p, nil
}

func (ps *ProviderStore) Remove(id string) error {
	db, workspaceID, cleanup, err := ps.openWorkspaceDB()
	if err != nil {
		return err
	}
	defer cleanup()
	if _, ok, err := db.ModelProviderByID(workspaceID, id); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("provider %q not found", id)
	}
	return db.DeleteModelProvider(workspaceID, id)
}

// ResolveEnv returns merged environment variables for a provider.
// Maps provider type + fields to the appropriate env var names.
func (ps *ProviderStore) ResolveEnv(id string) (map[string]string, error) {
	p, err := ps.Get(id)
	if err != nil {
		return nil, err
	}
	return ProviderEnv(*p), nil
}

// ResolveEnvForModel returns environment variables for a provider as consumed
// by the concrete agent CLI. The same workspace model account can be used by
// different CLIs, so runtime materialization is based on the agent model first,
// not only on the provider's stored type.
func (ps *ProviderStore) ResolveEnvForModel(id string, model entity.AgentModel) (map[string]string, error) {
	p, err := ps.Get(id)
	if err != nil {
		return nil, err
	}
	return ProviderEnvForModel(*p, model), nil
}

func ProviderEnv(p entity.APIProvider) map[string]string {
	env := make(map[string]string)
	for k, v := range p.Env {
		env[k] = v
	}
	switch p.Type {
	case "anthropic":
		applyClaudeCodeProviderEnv(env, p)
	case "openai":
		if p.APIKey != "" {
			env["OPENAI_API_KEY"] = p.APIKey
		}
		if p.BaseURL != "" {
			env["OPENAI_BASE_URL"] = p.BaseURL
		}
		if p.Model != "" {
			env["OPENAI_MODEL"] = p.Model
		}
	case "gemini":
		if p.APIKey != "" {
			env["GEMINI_API_KEY"] = p.APIKey
		}
		if p.BaseURL != "" {
			env["GOOGLE_API_BASE"] = p.BaseURL
		}
	case "cursor":
		if p.APIKey != "" {
			env["CURSOR_API_KEY"] = p.APIKey
		}
	}
	return env
}

func ProviderEnvForModel(p entity.APIProvider, model entity.AgentModel) map[string]string {
	env := make(map[string]string)
	for k, v := range p.Env {
		env[k] = v
	}
	switch entity.NormaliseModel(model) {
	case entity.ModelClaudeCode:
		applyClaudeCodeProviderEnv(env, p)
	case entity.ModelCodex:
		if p.APIKey != "" {
			env["CODEX_API_KEY"] = p.APIKey
			env["OPENAI_API_KEY"] = p.APIKey
		}
		if p.BaseURL != "" {
			env["OPENAI_BASE_URL"] = p.BaseURL
		}
		if p.Model != "" {
			env["CODEX_MODEL"] = p.Model
			env["OPENAI_MODEL"] = p.Model
		}
	case entity.ModelOpenCode:
		if p.APIKey != "" {
			env["OPENAI_API_KEY"] = p.APIKey
		}
		if p.BaseURL != "" {
			env["OPENAI_BASE_URL"] = p.BaseURL
		}
		if p.Model != "" {
			env["OPENAI_MODEL"] = p.Model
		}
	case entity.ModelGemini:
		if p.APIKey != "" {
			env["GEMINI_API_KEY"] = p.APIKey
		}
		if p.BaseURL != "" {
			env["GOOGLE_API_BASE"] = p.BaseURL
		}
		if p.Model != "" {
			env["GEMINI_MODEL"] = p.Model
			env["GOOGLE_MODEL"] = p.Model
		}
	case entity.ModelCursor:
		if p.APIKey != "" {
			env["CURSOR_API_KEY"] = p.APIKey
		}
		if p.Model != "" {
			env["CURSOR_MODEL"] = p.Model
		}
	default:
		return ProviderEnv(p)
	}
	return env
}

func applyClaudeCodeProviderEnv(env map[string]string, p entity.APIProvider) {
	if p.APIKey != "" {
		// Claude Code compatible gateways commonly use ANTHROPIC_AUTH_TOKEN,
		// while official Anthropic SDK-style clients use ANTHROPIC_API_KEY.
		// Set both so users do not need to understand that distinction.
		env["ANTHROPIC_AUTH_TOKEN"] = p.APIKey
		env["ANTHROPIC_API_KEY"] = p.APIKey
	}
	if p.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = p.BaseURL
	}
	if p.Model != "" {
		env["ANTHROPIC_MODEL"] = p.Model
		env["CLAUDE_MODEL"] = p.Model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = p.Model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = p.Model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = p.Model
	}
}

func (ps *ProviderStore) openWorkspaceDB() (controldb.Store, string, func(), error) {
	if ps.db != nil {
		workspaceID, err := ps.resolveWorkspaceID(ps.db)
		return ps.db, workspaceID, func() {}, err
	}
	db, err := controldb.OpenDefault()
	if err != nil {
		return nil, "", func() {}, err
	}
	workspaceID, err := ps.resolveWorkspaceID(db)
	if err != nil {
		_ = db.Close()
		return nil, "", func() {}, err
	}
	return db, workspaceID, func() { _ = db.Close() }, nil
}

func (ps *ProviderStore) resolveWorkspaceID(db controldb.Store) (string, error) {
	root, err := filepath.Abs(ps.root)
	if err != nil {
		root = ps.root
	}
	workspaces, err := db.ListWorkspaces()
	if err != nil {
		return "", err
	}
	for _, workspace := range workspaces {
		wsRoot, err := filepath.Abs(workspace.Root)
		if err != nil {
			wsRoot = workspace.Root
		}
		if filepath.Clean(wsRoot) == filepath.Clean(root) {
			return workspace.ID, nil
		}
	}
	return "", fmt.Errorf("workspace for root %q not found", ps.root)
}

func modelProviderFromDB(row controldb.ModelProvider) (entity.APIProvider, error) {
	env := map[string]string{}
	if strings.TrimSpace(row.EnvJSON) != "" {
		if err := json.Unmarshal([]byte(row.EnvJSON), &env); err != nil {
			return entity.APIProvider{}, err
		}
	}
	apiKey := ""
	if strings.TrimSpace(row.APIKey) != "" {
		opened, err := secretbox.OpenString(row.APIKey)
		if err != nil {
			return entity.APIProvider{}, err
		}
		apiKey = opened
	}
	return entity.APIProvider{
		ID:        row.ID,
		OwnerType: row.OwnerType,
		OwnerID:   row.OwnerID,
		Name:      row.Name,
		Type:      row.Type,
		BaseURL:   row.BaseURL,
		APIKey:    apiKey,
		Model:     row.Model,
		Env:       env,
	}, nil
}

func modelProviderToDB(workspaceID string, p entity.APIProvider) (controldb.ModelProvider, error) {
	env := p.Env
	if env == nil {
		env = map[string]string{}
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		return controldb.ModelProvider{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ownerType := strings.TrimSpace(p.OwnerType)
	ownerID := strings.TrimSpace(p.OwnerID)
	if ownerType == "" {
		ownerType = "workspace"
	}
	if ownerID == "" {
		ownerID = workspaceID
	}
	apiKey, err := secretbox.SealString(p.APIKey)
	if err != nil {
		return controldb.ModelProvider{}, err
	}
	return controldb.ModelProvider{
		ID:          strings.TrimSpace(p.ID),
		WorkspaceID: workspaceID,
		OwnerType:   ownerType,
		OwnerID:     ownerID,
		Name:        strings.TrimSpace(p.Name),
		Type:        strings.TrimSpace(p.Type),
		BaseURL:     strings.TrimSpace(p.BaseURL),
		APIKey:      apiKey,
		Model:       strings.TrimSpace(p.Model),
		EnvJSON:     string(envJSON),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}
