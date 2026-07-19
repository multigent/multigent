package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/multigent/multigent/internal/entity"
)

type skillRow struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description,omitempty"`
	Provenance  *entity.PlaybookObjectProvenance `json:"provenance,omitempty"`
}

type createSkillBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.st.ListSkills()
	if err != nil {
		s.serverError(w, err)
		return
	}
	workspaceID, _ := s.currentWorkspaceID()
	provenance, _ := s.playbookProvenanceMap(workspaceID, "skill")
	out := make([]skillRow, 0, len(skills))
	for _, sk := range skills {
		if sk == nil {
			continue
		}
		var prov *entity.PlaybookObjectProvenance
		if p, ok := provenance[playbookProvenanceKey("", sk.Name)]; ok {
			cp := p
			prov = &cp
		}
		out = append(out, skillRow{Name: sk.Name, Description: sk.Description, Provenance: prov})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleGetSkillDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := s.st.Skill(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		s.serverError(w, err)
		return
	}
	prompt, err := s.st.SkillPrompt(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":        sk.Name,
		"description": sk.Description,
		"prompt":      prompt,
		"dir":         s.st.SkillDir(name),
		"provenance":  s.playbookObjectProvenanceForRequest(r, "skill", "", name),
	})
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body createSkillBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if _, err := s.st.Skill(name); err == nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("skill %q already exists", name))
		return
	} else if !isNotFoundErr(err) {
		s.serverError(w, err)
		return
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	description := strings.TrimSpace(body.Description)
	if description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", description))
	}
	sb.WriteString("---\n\n")
	content := normalizeUploadedSkillContent(body.Content)
	if content == "" {
		sb.WriteString(fmt.Sprintf("# Skill: %s\n\n", name))
		sb.WriteString("Describe when to use this skill, the workflow to follow, and any constraints.\n")
	} else {
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
	}

	skillDir := s.st.SkillDir(name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		s.serverError(w, err)
		return
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(sb.String()), 0o644); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		Action:       "skill.create",
		ResourceType: "skill",
		ResourceID:   name,
		Summary:      "Skill created",
		After: map[string]any{
			"name":        name,
			"description": description,
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"skill": name,
	})
}

func normalizeUploadedSkillContent(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return content
	}
	return strings.TrimSpace(strings.TrimPrefix(rest[idx+4:], "\n"))
}

func (s *Server) handlePutSkillPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := s.st.Skill(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		s.serverError(w, err)
		return
	}

	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", sk.Name))
	if sk.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", sk.Description))
	}
	sb.WriteString("---\n")
	if body.Content != "" {
		sb.WriteString("\n")
		sb.WriteString(body.Content)
		if !strings.HasSuffix(body.Content, "\n") {
			sb.WriteString("\n")
		}
	}

	skillMD := filepath.Join(s.st.SkillDir(name), "SKILL.md")
	if err := os.WriteFile(skillMD, []byte(sb.String()), 0o644); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "skill", "", name)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Role / Team skill binding ─────────────────────────────────────────────────

type skillBindBody struct {
	Team   string `json:"team"`
	Role   string `json:"role"`
	Skill  string `json:"skill"`
	Action string `json:"action"` // "add" or "remove"
}

func (s *Server) handlePostRoleSkillBind(w http.ResponseWriter, r *http.Request) {
	var body skillBindBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	teamPath := strings.TrimSpace(body.Team)
	roleName := strings.TrimSpace(body.Role)
	skillName := strings.TrimSpace(body.Skill)
	action := strings.TrimSpace(body.Action)

	if teamPath == "" || roleName == "" || skillName == "" {
		s.jsonError(w, http.StatusBadRequest, "team, role, and skill are required")
		return
	}
	if action != "add" && action != "remove" {
		s.jsonError(w, http.StatusBadRequest, "action must be add or remove")
		return
	}

	role, err := s.st.Role(teamPath, roleName)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "role not found")
			return
		}
		s.serverError(w, err)
		return
	}

	if action == "add" {
		if _, err := s.st.Skill(skillName); err != nil {
			s.jsonError(w, http.StatusBadRequest, "skill not found: "+skillName)
			return
		}
		for _, sk := range role.Skills {
			if sk == skillName {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": role.Skills})
				return
			}
		}
		role.Skills = append(role.Skills, skillName)
	} else {
		filtered := make([]string, 0, len(role.Skills))
		for _, sk := range role.Skills {
			if sk != skillName {
				filtered = append(filtered, sk)
			}
		}
		role.Skills = filtered
	}

	if err := s.st.SaveRole(teamPath, roleName, role); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "role", teamPath, roleName)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": role.Skills})
}

func (s *Server) handlePostTeamSkillBind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Team   string `json:"team"`
		Skill  string `json:"skill"`
		Action string `json:"action"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	teamPath := strings.TrimSpace(body.Team)
	skillName := strings.TrimSpace(body.Skill)
	action := strings.TrimSpace(body.Action)

	if teamPath == "" || skillName == "" {
		s.jsonError(w, http.StatusBadRequest, "team and skill are required")
		return
	}
	if action != "add" && action != "remove" {
		s.jsonError(w, http.StatusBadRequest, "action must be add or remove")
		return
	}

	team, err := s.st.Team(teamPath)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "team not found")
			return
		}
		s.serverError(w, err)
		return
	}

	if action == "add" {
		if _, err := s.st.Skill(skillName); err != nil {
			s.jsonError(w, http.StatusBadRequest, "skill not found: "+skillName)
			return
		}
		for _, sk := range team.Skills {
			if sk == skillName {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": team.Skills})
				return
			}
		}
		team.Skills = append(team.Skills, skillName)
	} else {
		filtered := make([]string, 0, len(team.Skills))
		for _, sk := range team.Skills {
			if sk != skillName {
				filtered = append(filtered, sk)
			}
		}
		team.Skills = filtered
	}

	if err := s.st.SaveTeam(teamPath, team); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "team", "", teamPath)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": team.Skills})
}
