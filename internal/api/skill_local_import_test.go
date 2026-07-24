package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/store"
)

func TestLocalSkillPreviewAndImport(t *testing.T) {
	s, root := newProviderHandlerTestServer(t)
	s.st = store.NewDB(root, s.controlDB)
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(home, ".codex", "skills", "local-review")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir local skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: local-review\ndescription: Review local changes.\n---\n\nUse this local skill.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "check.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	systemDir := filepath.Join(home, ".codex", "skills", ".system", "hidden")
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		t.Fatalf("mkdir system skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(systemDir, "SKILL.md"), []byte("---\nname: hidden\n---\n\nHidden.\n"), 0o644); err != nil {
		t.Fatalf("write system skill: %v", err)
	}

	listRec := httptest.NewRecorder()
	s.handleListLocalSkills(listRec, providerTestRequest(http.MethodGet, "/api/v1/skills/local", "owner", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list local skills status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var preview struct {
		Candidates []localSkillCandidate `json:"candidates"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if len(preview.Candidates) != 1 {
		t.Fatalf("expected one visible skill, got %#v", preview.Candidates)
	}
	if preview.Candidates[0].Name != "local-review" || preview.Candidates[0].Source != "codex" || preview.Candidates[0].ID == "" {
		t.Fatalf("unexpected candidate: %#v", preview.Candidates[0])
	}

	importRec := httptest.NewRecorder()
	s.handleImportLocalSkills(importRec, providerTestRequest(http.MethodPost, "/api/v1/skills/local/import", "owner", localSkillImportBody{IDs: []string{preview.Candidates[0].ID}}))
	if importRec.Code != http.StatusOK {
		t.Fatalf("import local skills status=%d body=%s", importRec.Code, importRec.Body.String())
	}
	importedSkill := s.st.SkillDir("local-review")
	raw, err := os.ReadFile(filepath.Join(importedSkill, "SKILL.md"))
	if err != nil {
		t.Fatalf("read imported SKILL.md: %v", err)
	}
	if !strings.Contains(string(raw), "source_type: local-sync") || !strings.Contains(string(raw), "Use this local skill.") {
		t.Fatalf("imported skill metadata/body unexpected:\n%s", string(raw))
	}
	if _, err := os.Stat(filepath.Join(importedSkill, "scripts", "check.sh")); err != nil {
		t.Fatalf("script was not copied: %v", err)
	}
}
