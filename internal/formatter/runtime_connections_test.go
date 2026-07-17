package formatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multigent/multigent/internal/ctxbuild"
)

func testMergedContext() *ctxbuild.MergedContext {
	return &ctxbuild.MergedContext{
		Layers: []ctxbuild.ContextLayer{
			{Source: "agency", Content: "# Agency\n\nWork carefully."},
		},
	}
}

func TestCodexFormatterIncludesRuntimeConnectionGuide(t *testing.T) {
	outDir := t.TempDir()
	if err := (&codexFormatter{}).Format(testMergedContext(), outDir); err != nil {
		t.Fatalf("format: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(outDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"## Runtime Connections",
		"MULTIGENT_CONNECTIONS_FILE",
		"MULTIGENT_TOOLS_FILE",
		"recommendedAdapter",
		"mga runtime tools --format table",
		"mga runtime connections --format table",
		"mga runtime action --connection <alias>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, text)
		}
	}
}

func TestClaudeFormatterImportsRuntimeConnectionGuide(t *testing.T) {
	outDir := t.TempDir()
	if err := (&claudeCodeFormatter{}).Format(testMergedContext(), outDir); err != nil {
		t.Fatalf("format: %v", err)
	}
	claudeBody, err := os.ReadFile(filepath.Join(outDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claudeBody), "@.multigent/context/runtime-connections.md") {
		t.Fatalf("CLAUDE.md missing runtime import:\n%s", string(claudeBody))
	}
	guideBody, err := os.ReadFile(filepath.Join(outDir, ".multigent", "context", "runtime-connections.md"))
	if err != nil {
		t.Fatalf("read runtime guide: %v", err)
	}
	if !strings.Contains(string(guideBody), "MULTIGENT_AGENT_TOKEN") {
		t.Fatalf("runtime guide missing token env:\n%s", string(guideBody))
	}
}
