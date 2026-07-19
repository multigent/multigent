package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	playbookstore "github.com/multigent/multigent/internal/playbook"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	playbookInstallsTable   = "playbook_installs"
	playbookProvenanceTable = "playbook_object_provenance"
)

func (s *Server) handleListPlaybookTemplates(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	templates := playbookstore.Templates(locale)
	for i := range templates {
		templates[i] = playbookTemplateSummary(templates[i])
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"templates": templates})
}

func (s *Server) handleListPlaybookInstalls(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	installs, err := s.listPlaybookInstalls(workspaceID)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"installs": installs})
}

func (s *Server) handleGetPlaybookTemplate(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !s.checkWorkspaceAccess(w, r, workspaceID) {
		return
	}
	locale := strings.TrimSpace(r.URL.Query().Get("locale"))
	tmpl, ok := playbookstore.Template(r.PathValue("playbookId"), locale)
	if !ok {
		s.jsonError(w, http.StatusNotFound, "playbook template not found")
		return
	}
	_ = json.NewEncoder(w).Encode(tmpl)
}

func playbookTemplateSummary(tmpl entity.PlaybookTemplate) entity.PlaybookTemplate {
	for i := range tmpl.Roles {
		tmpl.Roles[i].Prompt = ""
	}
	for i := range tmpl.Skills {
		tmpl.Skills[i].Body = ""
	}
	return tmpl
}

type installPlaybookRequest struct {
	Locale string `json:"locale"`
}

type installPlaybookResponse struct {
	Install          entity.PlaybookInstall `json:"install"`
	AlreadyInstalled bool                   `json:"alreadyInstalled"`
}

func (s *Server) handleInstallPlaybookTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		s.serverError(w, err)
		return
	}
	var body installPlaybookRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := s.readJSON(w, r, &body); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
			return
		}
	}
	locale := strings.TrimSpace(body.Locale)
	if locale == "" {
		locale = strings.TrimSpace(r.URL.Query().Get("locale"))
	}
	tmpl, ok := playbookstore.Template(r.PathValue("playbookId"), locale)
	if !ok {
		s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, "playbook template not found")
		return
	}
	if existing, ok, err := s.playbookInstallByPlaybook(workspaceID, tmpl.ID); err != nil {
		s.serverError(w, err)
		return
	} else if ok {
		_ = json.NewEncoder(w).Encode(installPlaybookResponse{Install: existing, AlreadyInstalled: true})
		return
	}

	install, err := s.installPlaybookTemplate(workspaceID, tmpl, currentUsername(s.currentUser(r)))
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "playbook.install",
		ResourceType: "playbook",
		ResourceID:   tmpl.ID,
		Summary:      "Playbook installed",
		After: map[string]any{
			"playbookId": tmpl.ID,
			"objects":    install.Objects,
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(installPlaybookResponse{Install: install})
}

func (s *Server) installPlaybookTemplate(workspaceID string, tmpl entity.PlaybookTemplate, username string) (entity.PlaybookInstall, error) {
	now := time.Now().UTC()
	install := entity.PlaybookInstall{
		ID:              fmt.Sprintf("pbi-%d", now.UnixNano()),
		PlaybookID:      tmpl.ID,
		PlaybookName:    tmpl.Name,
		TemplateVersion: tmpl.Version,
		Locale:          tmpl.Locale,
		CreatedBy:       username,
		CreatedAt:       now,
	}
	addObject := func(obj entity.PlaybookInstalledObject) error {
		install.Objects = append(install.Objects, obj)
		return s.savePlaybookObjectProvenance(workspaceID, entity.PlaybookObjectProvenance{
			ObjectType:      obj.Type,
			ObjectID:        obj.ID,
			ParentID:        obj.ParentID,
			PlaybookID:      tmpl.ID,
			PlaybookName:    tmpl.Name,
			TemplateVersion: tmpl.Version,
			InstallID:       install.ID,
			InstalledBy:     username,
			InstalledAt:     now,
			Status:          obj.Status,
		})
	}

	for _, sk := range tmpl.Skills {
		status, err := s.ensurePlaybookSkill(sk)
		if err != nil {
			return install, err
		}
		if err := addObject(entity.PlaybookInstalledObject{Type: "skill", ID: sk.ID, Name: sk.Name, Status: status}); err != nil {
			return install, err
		}
	}

	seenTeams := map[string]bool{}
	for _, roleTpl := range tmpl.Roles {
		if !seenTeams[roleTpl.Team] {
			status, err := s.ensurePlaybookTeam(roleTpl.Team, tmpl)
			if err != nil {
				return install, err
			}
			if err := addObject(entity.PlaybookInstalledObject{Type: "team", ID: roleTpl.Team, Name: roleTpl.Team, Status: status}); err != nil {
				return install, err
			}
			seenTeams[roleTpl.Team] = true
		}
		status, err := s.ensurePlaybookRole(roleTpl)
		if err != nil {
			return install, err
		}
		if err := addObject(entity.PlaybookInstalledObject{Type: "role", ID: roleTpl.Role, Name: roleTpl.Name, ParentID: roleTpl.Team, Status: status}); err != nil {
			return install, err
		}
	}

	wfStore := workflowstore.NewStore(s.controlDB, workspaceID)
	for _, wfTpl := range tmpl.Workflows {
		defTpl := wfTpl.Definition
		def := entity.WorkflowDefinition{
			ID:          entity.NewWorkflowID(),
			Name:        wfTpl.Name,
			Description: wfTpl.Description,
			Version:     1,
			Scope:       "workspace",
			StartStepID: defTpl.StartStepID,
			Steps:       defTpl.Steps,
			Edges:       defTpl.Edges,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if def.Name == "" {
			def.Name = defTpl.Name
		}
		if def.Description == "" {
			def.Description = defTpl.Description
		}
		if err := wfStore.SaveDefinition(&def); err != nil {
			return install, err
		}
		if err := addObject(entity.PlaybookInstalledObject{Type: "workflow", ID: def.ID, Name: def.Name, Status: "created"}); err != nil {
			return install, err
		}
	}

	raw, err := json.Marshal(install)
	if err != nil {
		return install, err
	}
	if err := s.controlDB.UpsertRecord(playbookInstallsTable, workspaceID, []string{install.ID}, string(raw)); err != nil {
		return install, err
	}
	return install, nil
}

func (s *Server) ensurePlaybookSkill(sk entity.PlaybookSkillTemplate) (string, error) {
	name := strings.TrimSpace(sk.ID)
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		return "", err
	}
	if _, err := s.st.Skill(name); err == nil {
		return "existing", nil
	} else if !isNotFoundErr(err) {
		return "", err
	}
	body := strings.TrimSpace(sk.Body)
	if body == "" {
		body = fmt.Sprintf("# Skill: %s\n\n%s\n", sk.Name, strings.TrimSpace(sk.Description))
	}
	var sb strings.Builder
	if strings.HasPrefix(body, "---") {
		sb.WriteString(body)
	} else {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("name: %s\n", name))
		if strings.TrimSpace(sk.Description) != "" {
			sb.WriteString(fmt.Sprintf("description: %q\n", strings.TrimSpace(sk.Description)))
		}
		sb.WriteString("---\n\n")
		sb.WriteString(body)
	}
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}
	dir := s.st.SkillDir(name)
	if strings.Contains(sk.Source, "garrytan/gstack") {
		if err := playbookstore.CopyGstackSkillAssets(name, dir); err != nil {
			return "", err
		}
		return "created", nil
	}
	if category, assetName, ok := mattPocockAssetRef(sk.Source); ok {
		if err := playbookstore.CopyMattPocockSkillAssets(category, assetName, dir); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return "created", nil
}

func mattPocockAssetRef(source string) (string, string, bool) {
	_, rest, ok := strings.Cut(source, "mattpocock/skills:")
	if !ok {
		return "", "", false
	}
	category, assetName, ok := strings.Cut(strings.TrimSpace(rest), "/")
	if !ok {
		return "", "", false
	}
	category = strings.TrimSpace(category)
	assetName = strings.TrimSpace(assetName)
	if category == "" || assetName == "" {
		return "", "", false
	}
	return category, assetName, true
}

func (s *Server) ensurePlaybookTeam(teamName string, tmpl entity.PlaybookTemplate) (string, error) {
	teamName = strings.TrimSpace(teamName)
	if err := validateWorkspaceObjectName("team", teamName); err != nil {
		return "", err
	}
	if _, err := s.st.Team(teamName); err == nil {
		return "existing", nil
	} else if !isNotFoundErr(err) {
		return "", err
	}
	team := &entity.Team{
		Name:        teamName,
		Description: fmt.Sprintf("Installed from playbook: %s", tmpl.Name),
	}
	if err := s.st.SaveTeam(teamName, team); err != nil {
		return "", err
	}
	_ = s.st.SaveTeamPrompt(teamName, fmt.Sprintf("# Team: %s\n\n%s\n", teamName, team.Description))
	return "created", nil
}

func (s *Server) ensurePlaybookRole(roleTpl entity.PlaybookRoleTemplate) (string, error) {
	teamName := strings.TrimSpace(roleTpl.Team)
	roleName := strings.TrimSpace(roleTpl.Role)
	if err := validateWorkspaceObjectName("role", roleName); err != nil {
		return "", err
	}
	if _, err := s.st.Role(teamName, roleName); err == nil {
		return "existing", nil
	} else if !isNotFoundErr(err) {
		return "", err
	}
	role := &entity.Role{
		Name:        roleName,
		Description: strings.TrimSpace(roleTpl.Description),
		Skills:      append([]string(nil), roleTpl.Skills...),
	}
	if err := s.st.SaveRole(teamName, roleName, role); err != nil {
		return "", err
	}
	prompt := strings.TrimSpace(roleTpl.Prompt)
	if prompt == "" {
		prompt = fmt.Sprintf("# Role: %s\n\n%s\n", roleTpl.Name, strings.TrimSpace(roleTpl.Description))
	}
	_ = s.st.SaveRolePrompt(teamName, roleName, prompt)
	return "created", nil
}

func (s *Server) listPlaybookInstalls(workspaceID string) ([]entity.PlaybookInstall, error) {
	recs, err := s.controlDB.ListRecords(playbookInstallsTable, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]entity.PlaybookInstall, 0, len(recs))
	for _, rec := range recs {
		var install entity.PlaybookInstall
		if err := json.Unmarshal([]byte(rec.Payload), &install); err == nil {
			out = append(out, install)
		}
	}
	return out, nil
}

func (s *Server) playbookInstallByPlaybook(workspaceID, playbookID string) (entity.PlaybookInstall, bool, error) {
	installs, err := s.listPlaybookInstalls(workspaceID)
	if err != nil {
		return entity.PlaybookInstall{}, false, err
	}
	for _, install := range installs {
		if install.PlaybookID == playbookID {
			return install, true, nil
		}
	}
	return entity.PlaybookInstall{}, false, nil
}

func (s *Server) savePlaybookObjectProvenance(workspaceID string, prov entity.PlaybookObjectProvenance) error {
	raw, err := json.Marshal(prov)
	if err != nil {
		return err
	}
	key := []string{prov.ObjectType, prov.ParentID, prov.ObjectID, prov.InstallID}
	return s.controlDB.UpsertRecord(playbookProvenanceTable, workspaceID, key, string(raw))
}

func (s *Server) markPlaybookObjectCustomized(r *http.Request, objectType, parentID, objectID string) {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return
	}
	provenance, err := s.playbookProvenanceMap(workspaceID, objectType)
	if err != nil {
		return
	}
	prov, ok := provenance[playbookProvenanceKey(parentID, objectID)]
	if !ok || prov.Customized {
		return
	}
	prov.Customized = true
	prov.CustomizedBy = requestUsername(r)
	prov.CustomizedAt = time.Now().UTC()
	if err := s.savePlaybookObjectProvenance(workspaceID, prov); err != nil {
		// Customization provenance is advisory; never fail the user's actual edit.
		return
	}
}

func (s *Server) playbookProvenanceMap(workspaceID, objectType string) (map[string]entity.PlaybookObjectProvenance, error) {
	recs, err := s.controlDB.ListRecords(playbookProvenanceTable, workspaceID, []string{objectType})
	if err != nil {
		return nil, err
	}
	out := map[string]entity.PlaybookObjectProvenance{}
	for _, rec := range recs {
		var prov entity.PlaybookObjectProvenance
		if err := json.Unmarshal([]byte(rec.Payload), &prov); err != nil {
			continue
		}
		key := playbookProvenanceKey(prov.ParentID, prov.ObjectID)
		prev, ok := out[key]
		if !ok || prov.InstalledAt.After(prev.InstalledAt) {
			out[key] = prov
		}
	}
	return out, nil
}

func playbookProvenanceKey(parentID, objectID string) string {
	return strings.TrimSpace(parentID) + "\x00" + strings.TrimSpace(objectID)
}

func (s *Server) playbookObjectProvenanceForRequest(_ *http.Request, objectType, parentID, objectID string) *entity.PlaybookObjectProvenance {
	workspaceID, err := s.currentWorkspaceID()
	if err != nil {
		return nil
	}
	provenance, err := s.playbookProvenanceMap(workspaceID, objectType)
	if err != nil {
		return nil
	}
	if p, ok := provenance[playbookProvenanceKey(parentID, objectID)]; ok {
		return &p
	}
	return nil
}
