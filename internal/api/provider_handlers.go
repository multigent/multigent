package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/store"
)

// providerBody is the JSON request struct for creating/updating providers.
// Separate from entity.APIProvider because APIKey uses json:"-" on the entity
// to prevent leaking in responses, but we need to accept it in requests.
type providerBody struct {
	OwnerType string            `json:"ownerType"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	BaseURL   string            `json:"baseUrl"`
	APIKey    string            `json:"apiKey"`
	Model     string            `json:"model"`
	Models    []string          `json:"models,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type ccSwitchProviderRow struct {
	ID             string
	AppType        string
	Name           string
	SettingsConfig string
	IsCurrent      int
}

type ccSwitchProviderPreview struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CLI       string `json:"cli"`
	Type      string `json:"type"`
	BaseURL   string `json:"baseUrl,omitempty"`
	Model     string `json:"model,omitempty"`
	HasKey    bool   `json:"hasKey"`
	IsCurrent bool   `json:"isCurrent"`
}

type ccSwitchImportBody struct {
	IDs []string `json:"ids"`
}

func (b providerBody) toEntity() entity.APIProvider {
	return entity.APIProvider{
		OwnerType: strings.TrimSpace(b.OwnerType),
		Name:      strings.TrimSpace(b.Name),
		Type:      strings.TrimSpace(b.Type),
		BaseURL:   strings.TrimSpace(b.BaseURL),
		APIKey:    strings.TrimSpace(b.APIKey),
		Model:     strings.TrimSpace(b.Model),
		Models:    cleanProviderModels(b.Models),
		Env:       b.Env,
	}
}

func (s *Server) providerStore() *store.ProviderStore {
	return store.NewProviderStoreWithDB(s.root, s.controlDB)
}

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return
	}
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	agentScoped := project != "" || agent != ""
	if agentScoped {
		if project == "" || agent == "" {
			s.jsonError(w, http.StatusBadRequest, "project and agent are required together")
			return
		}
		if !s.agentExistsInProject(project, agent) {
			s.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		if !s.canManageAgentConfig(r, project, agent) {
			s.jsonError(w, http.StatusForbidden, "agent management access required")
			return
		}
	}
	items, err := s.providerStore().List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		if agentScoped {
			if !s.canUseModelProviderForAgent(r, p, project, agent) {
				continue
			}
		} else if !canViewModelProvider(p) {
			continue
		}
		out = append(out, providerToJSON(p))
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAddProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceAdmin(w, r)
	if err != nil {
		return
	}
	var body providerBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	prov := body.toEntity()
	if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
		return
	}
	if prov.Name == "" {
		s.jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.providerStore().Add(prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.create",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      "Model provider created",
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(providerToJSON(*p))
}

func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceAdmin(w, r)
	if err != nil {
		return
	}
	id := r.PathValue("id")
	var body providerBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ps := s.providerStore()
	existing, err := ps.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	if !s.canManageModelProvider(r, *existing) {
		s.jsonError(w, http.StatusForbidden, "model provider management access required")
		return
	}
	prov := body.toEntity()
	// If no new key provided, keep the existing one.
	if prov.APIKey == "" {
		prov.APIKey = existing.APIKey
	}
	p, err := ps.Update(id, prov)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.update",
		ResourceType: "model_provider",
		ResourceID:   p.ID,
		Summary:      "Model provider updated",
		Before:       modelProviderAuditPayload(*existing),
		After:        modelProviderAuditPayload(*p),
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(providerToJSON(*p))
}

func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceAdmin(w, r)
	if err != nil {
		return
	}
	id := r.PathValue("id")
	ps := s.providerStore()
	existing, err := ps.Get(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	if !s.canManageModelProvider(r, *existing) {
		s.jsonError(w, http.StatusForbidden, "model provider management access required")
		return
	}
	if err := ps.Remove(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	clearedAgents, err := s.clearDeletedModelProviderRefs(id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  workspaceID,
		Action:       "model_provider.delete",
		ResourceType: "model_provider",
		ResourceID:   id,
		Summary:      "Model provider deleted",
		Before:       modelProviderAuditPayload(*existing),
		After:        map[string]any{"clearedAgents": clearedAgents},
		Request:      r,
	})
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "clearedAgents": clearedAgents})
}

func (s *Server) handleListCCSwitchProviders(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	rows, dbPath, err := listCCSwitchProviderRows()
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"available": false,
			"providers": []ccSwitchProviderPreview{},
			"searched":  ccSwitchDBCandidates(),
			"error":     err.Error(),
		})
		return
	}
	out := make([]ccSwitchProviderPreview, 0, len(rows))
	for _, row := range rows {
		prov, cli, err := convertCCSwitchModelProvider(row)
		if err != nil {
			continue
		}
		out = append(out, ccSwitchProviderPreview{
			ID:        row.ID,
			Name:      prov.Name,
			CLI:       cli,
			Type:      prov.Type,
			BaseURL:   prov.BaseURL,
			Model:     prov.Model,
			HasKey:    prov.APIKey != "" || len(prov.Env) > 0,
			IsCurrent: row.IsCurrent == 1,
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"available": true,
		"dbPath":    dbPath,
		"providers": out,
	})
}

func (s *Server) handleImportCCSwitchProviders(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.modelProviderWorkspaceAdmin(w, r)
	if err != nil {
		return
	}
	rows, _, err := listCCSwitchProviderRows()
	if err != nil {
		s.jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	var body ccSwitchImportBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	want := map[string]bool{}
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		s.jsonError(w, http.StatusBadRequest, "ids are required")
		return
	}
	existing, err := s.providerStore().List()
	if err != nil {
		s.serverError(w, err)
		return
	}
	existingByNameType := map[string]bool{}
	for _, provider := range existing {
		existingByNameType[providerNameTypeKey(provider)] = true
	}
	imported := []map[string]any{}
	skipped := []map[string]string{}
	for _, row := range rows {
		if !want[row.ID] {
			continue
		}
		prov, cli, err := convertCCSwitchModelProvider(row)
		if err != nil {
			skipped = append(skipped, map[string]string{"id": row.ID, "name": row.Name, "reason": err.Error()})
			continue
		}
		if !s.prepareNewModelProvider(w, r, workspaceID, &prov) {
			return
		}
		key := providerNameTypeKey(prov)
		if existingByNameType[key] {
			skipped = append(skipped, map[string]string{"id": row.ID, "name": prov.Name, "reason": "already imported"})
			continue
		}
		created, err := s.providerStore().Add(prov)
		if err != nil {
			skipped = append(skipped, map[string]string{"id": row.ID, "name": prov.Name, "reason": err.Error()})
			continue
		}
		existingByNameType[key] = true
		s.auditLog(auditLogInput{
			WorkspaceID:  workspaceID,
			Action:       "model_provider.import_cc_switch",
			ResourceType: "model_provider",
			ResourceID:   created.ID,
			Summary:      "Model provider imported from cc-switch",
			After:        modelProviderAuditPayload(*created),
			Request:      r,
		})
		payload := providerToJSON(*created)
		payload["cli"] = cli
		imported = append(imported, payload)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"imported": imported,
		"skipped":  skipped,
	})
}

func (s *Server) clearDeletedModelProviderRefs(providerID string) ([]string, error) {
	if s.st == nil {
		return []string{}, nil
	}
	projects, err := s.st.ListProjects()
	if err != nil {
		return nil, err
	}
	cleared := []string{}
	for _, project := range projects {
		if project == nil || strings.TrimSpace(project.Name) == "" {
			continue
		}
		agents, err := s.st.ListAgents(project.Name)
		if err != nil {
			return nil, err
		}
		for _, agent := range agents {
			if agent == nil || strings.TrimSpace(agent.Name) == "" {
				continue
			}
			meta, err := s.st.AgentMeta(project.Name, agent.Name)
			if err != nil {
				return nil, err
			}
			if meta.Provider != providerID {
				continue
			}
			meta.Provider = ""
			if err := s.st.SaveAgentMeta(project.Name, agent.Name, meta); err != nil {
				return nil, err
			}
			cleared = append(cleared, project.Name+"/"+agent.Name)
		}
	}
	return cleared, nil
}

func (s *Server) modelProviderWorkspaceMember(w http.ResponseWriter, r *http.Request) (string, error) {
	if !s.checkCurrentWorkspaceAccess(w, r) {
		return "", http.ErrAbortHandler
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return "", err
	}
	return workspaceID, nil
}

func (s *Server) modelProviderWorkspaceAdmin(w http.ResponseWriter, r *http.Request) (string, error) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return "", http.ErrAbortHandler
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return "", err
	}
	return workspaceID, nil
}

func (s *Server) prepareNewModelProvider(w http.ResponseWriter, r *http.Request, workspaceID string, p *entity.APIProvider) bool {
	if !s.canAdminWorkspace(r, workspaceID) {
		s.jsonError(w, http.StatusForbidden, "workspace admin access required")
		return false
	}
	p.OwnerType = ConnectionOwnerWorkspace
	p.OwnerID = workspaceID
	return true
}

func canViewModelProvider(p entity.APIProvider) bool {
	switch p.OwnerType {
	case "", ConnectionOwnerWorkspace:
		return true
	default:
		return false
	}
}

func (s *Server) canManageModelProvider(r *http.Request, p entity.APIProvider) bool {
	switch p.OwnerType {
	case "", ConnectionOwnerWorkspace:
		return s.canAdminWorkspace(r, p.OwnerID)
	default:
		return false
	}
}

func providerToJSON(p entity.APIProvider) map[string]any {
	authMethod := store.ProviderAuthMethod(p)
	authConfigured := store.ProviderAuthConfigured(p)
	out := map[string]any{
		"id":             p.ID,
		"ownerType":      p.OwnerType,
		"ownerId":        p.OwnerID,
		"name":           p.Name,
		"type":           p.Type,
		"baseUrl":        p.BaseURL,
		"model":          p.Model,
		"models":         cleanProviderModels(p.Models),
		"hasKey":         p.APIKey != "",
		"authMethod":     authMethod,
		"authConfigured": authConfigured,
	}
	if len(p.Env) > 0 {
		out["env"] = p.Env
	}
	return out
}

func modelProviderAuditPayload(p entity.APIProvider) map[string]any {
	return map[string]any{
		"id":        p.ID,
		"ownerType": p.OwnerType,
		"ownerId":   p.OwnerID,
		"name":      p.Name,
		"type":      p.Type,
		"baseUrl":   p.BaseURL,
		"model":     p.Model,
		"models":    cleanProviderModels(p.Models),
		"hasKey":    p.APIKey != "",
	}
}

func cleanProviderModels(models []string) []string {
	out := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	return out
}

func listCCSwitchProviderRows() ([]ccSwitchProviderRow, string, error) {
	dbPath := findCCSwitchDB()
	if dbPath == "" {
		return nil, "", fmt.Errorf("cc-switch database not found")
	}
	rows, err := queryCCSwitchDB(dbPath)
	return rows, dbPath, err
}

func queryCCSwitchDB(dbPath string) ([]ccSwitchProviderRow, error) {
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open cc-switch db: %w", err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT id, app_type, name, settings_config, is_current FROM providers")
	if err != nil {
		return nil, fmt.Errorf("query cc-switch db: %w", err)
	}
	defer rows.Close()
	out := []ccSwitchProviderRow{}
	for rows.Next() {
		var row ccSwitchProviderRow
		if err := rows.Scan(&row.ID, &row.AppType, &row.Name, &row.SettingsConfig, &row.IsCurrent); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func convertCCSwitchModelProvider(row ccSwitchProviderRow) (entity.APIProvider, string, error) {
	var settings map[string]any
	if err := json.Unmarshal([]byte(row.SettingsConfig), &settings); err != nil {
		return entity.APIProvider{}, "", fmt.Errorf("invalid settings_config JSON: %w", err)
	}
	prov := entity.APIProvider{
		Name: strings.TrimSpace(row.Name),
	}
	if prov.Name == "" {
		prov.Name = row.AppType + " provider"
	}
	switch strings.ToLower(strings.TrimSpace(row.AppType)) {
	case "claude":
		prov.Type = "anthropic"
		if err := fillClaudeCCSwitchProvider(&prov, settings); err != nil {
			return entity.APIProvider{}, "", err
		}
		return prov, "claudecode", nil
	case "codex":
		prov.Type = "openai"
		if err := fillCodexCCSwitchProvider(&prov, settings); err != nil {
			return entity.APIProvider{}, "", err
		}
		return prov, "codex", nil
	default:
		return entity.APIProvider{}, "", fmt.Errorf("unsupported app_type %q", row.AppType)
	}
}

func fillClaudeCCSwitchProvider(prov *entity.APIProvider, settings map[string]any) error {
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		return fmt.Errorf("no env in settings_config")
	}
	if key := firstStringFromMap(env, "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"); key != "" {
		prov.APIKey = key
	}
	if baseURL := firstStringFromMap(env, "ANTHROPIC_BASE_URL"); baseURL != "" {
		prov.BaseURL = baseURL
	}
	if model := firstStringFromMap(env, "ANTHROPIC_MODEL", "CLAUDE_MODEL"); model != "" {
		prov.Model = model
		prov.Models = []string{model}
	}
	extra := map[string]string{}
	known := map[string]bool{
		"ANTHROPIC_AUTH_TOKEN": true,
		"ANTHROPIC_API_KEY":    true,
		"ANTHROPIC_BASE_URL":   true,
		"ANTHROPIC_MODEL":      true,
		"CLAUDE_MODEL":         true,
	}
	for key, value := range env {
		if known[key] {
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			extra[key] = s
		}
	}
	if len(extra) > 0 {
		prov.Env = extra
	}
	if prov.APIKey == "" && len(prov.Env) == 0 {
		return fmt.Errorf("no API key or env found")
	}
	return nil
}

func fillCodexCCSwitchProvider(prov *entity.APIProvider, settings map[string]any) error {
	if auth, ok := settings["auth"].(map[string]any); ok {
		prov.APIKey = firstStringFromMap(auth, "OPENAI_API_KEY")
	}
	if cfg, ok := settings["config"].(string); ok {
		prov.BaseURL, prov.Model = parseCodexProviderConfig(cfg)
	}
	if env, ok := settings["env"].(map[string]any); ok {
		if prov.APIKey == "" {
			prov.APIKey = firstStringFromMap(env, "OPENAI_API_KEY")
		}
		if prov.BaseURL == "" {
			prov.BaseURL = firstStringFromMap(env, "OPENAI_BASE_URL", "OPENAI_API_BASE")
		}
		if prov.Model == "" {
			prov.Model = firstStringFromMap(env, "OPENAI_MODEL")
		}
	}
	if prov.APIKey == "" {
		return fmt.Errorf("no OPENAI_API_KEY found")
	}
	if prov.Model != "" {
		prov.Models = []string{prov.Model}
	}
	return nil
}

func parseCodexProviderConfig(config string) (baseURL, model string) {
	for _, line := range strings.Split(config, "\n") {
		key, value, ok := parseSimpleTOMLKV(line)
		if !ok {
			continue
		}
		switch key {
		case "base_url":
			if baseURL == "" {
				baseURL = value
			}
		case "model":
			if model == "" {
				model = value
			}
		}
	}
	return baseURL, model
}

func parseSimpleTOMLKV(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
		return "", "", false
	}
	key, value, ok = strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return key, value, key != ""
}

func firstStringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findCCSwitchDB() string {
	for _, candidate := range ccSwitchDBCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func ccSwitchDBCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{filepath.Join(home, ".cc-switch", "cc-switch.db")}
	switch runtime.GOOS {
	case "linux":
		dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		candidates = append(candidates, filepath.Join(dataHome, "cc-switch", "cc-switch.db"))
	case "darwin":
		candidates = append(candidates, filepath.Join(home, "Library", "Application Support", "cc-switch", "cc-switch.db"))
	}
	return candidates
}

func providerNameTypeKey(provider entity.APIProvider) string {
	return strings.ToLower(strings.TrimSpace(provider.Name)) + "\x00" + strings.ToLower(strings.TrimSpace(provider.Type))
}
