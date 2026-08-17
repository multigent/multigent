package contextpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/store"
)

func TestImportManualCreatesDocAndAgentBinding(t *testing.T) {
	root := t.TempDir()
	cs := NewStore(root)
	res, err := cs.ImportManual(ImportManualInput{
		Title:       "Legacy Session",
		Content:     "# Notes\nold context",
		SourceName:  "session.jsonl",
		CreatedBy:   "admin",
		Project:     "demo",
		BindScope:   ScopeAgent,
		BindScopeID: "demo/Lina",
		Required:    true,
	})
	if err != nil {
		t.Fatalf("ImportManual failed: %v", err)
	}
	if res.Doc == nil || res.Doc.ID == "" {
		t.Fatalf("expected managed doc")
	}
	if res.Binding == nil || res.Binding.ScopeID != "demo/Lina" {
		t.Fatalf("expected agent binding, got %#v", res.Binding)
	}
	views, err := cs.ListBindingViews(AgentScopes("demo", "Lina"))
	if err != nil {
		t.Fatalf("ListBindingViews failed: %v", err)
	}
	if len(views) != 1 || views[0].Artifact.DocID != res.Doc.ID {
		t.Fatalf("unexpected views: %#v", views)
	}
	layer, err := BuildAgentContextLayer(root, "demo", "Lina")
	if err != nil {
		t.Fatalf("BuildAgentContextLayer failed: %v", err)
	}
	if !strings.Contains(layer, "Legacy Session") || !strings.Contains(layer, "mga context read") {
		t.Fatalf("layer missing context instructions:\n%s", layer)
	}
}

func TestAddBindingCreatesArtifactForExistingDoc(t *testing.T) {
	root := t.TempDir()
	ds := store.NewDocsStore(root)
	doc := &store.DocEntry{
		Title:       "Project Brief",
		Index:       "projects/demo/context",
		CreatedBy:   "admin",
		Description: "Important project background.",
	}
	if err := ds.AddManagedContent(doc, "# Brief\nread me", "brief.md"); err != nil {
		t.Fatalf("AddManagedContent failed: %v", err)
	}

	cs := NewStore(root)
	binding, err := cs.AddBinding(Binding{
		DocID:     doc.ID,
		ScopeType: ScopeAgent,
		ScopeID:   "demo/Lina",
		Required:  true,
	})
	if err != nil {
		t.Fatalf("AddBinding failed: %v", err)
	}
	if binding.ArtifactID == "" {
		t.Fatalf("expected artifact id on binding: %#v", binding)
	}

	views, err := cs.ListBindingViews(AgentScopes("demo", "Lina"))
	if err != nil {
		t.Fatalf("ListBindingViews failed: %v", err)
	}
	if len(views) != 1 || views[0].Artifact.Title != "Project Brief" {
		t.Fatalf("unexpected binding views: %#v", views)
	}
	if _, err := cs.AddBinding(Binding{DocID: doc.ID, ScopeType: ScopeAgent, ScopeID: "demo/Lina"}); err == nil {
		t.Fatalf("expected duplicate binding error")
	}
}

func TestImportFileCreatesKnowledgeDocAndBinding(t *testing.T) {
	root := t.TempDir()
	fileDir := filepath.Join(root, ".multigent", "files", "sessions")
	if err := os.MkdirAll(fileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileDir, "codex.jsonl"), []byte(`{"type":"message","text":"old session"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := NewStore(root).ImportFile(ImportFileInput{
		FilePath:    "sessions/codex.jsonl",
		Title:       "Codex Session",
		CreatedBy:   "admin",
		Project:     "demo",
		BindScope:   ScopeProject,
		BindScopeID: "demo",
	})
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if res.Asset.Kind != "file" || res.Doc == nil || res.Doc.ID == "" {
		t.Fatalf("unexpected import result: %#v", res)
	}
	views, err := NewStore(root).ListBindingViews([]ScopeRef{{Type: ScopeProject, ID: "demo"}})
	if err != nil {
		t.Fatalf("ListBindingViews failed: %v", err)
	}
	if len(views) != 1 || views[0].Artifact.DocID != res.Doc.ID {
		t.Fatalf("unexpected views: %#v", views)
	}
}
