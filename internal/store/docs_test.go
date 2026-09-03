package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryDocsRanksChinesePhraseAndReturnsSnippet(t *testing.T) {
	root := t.TempDir()
	ds := NewDocsStore(root)

	ceoPath := filepath.Join(root, "ceo-notes.md")
	if err := os.WriteFile(ceoPath, []byte("2026年8月 CEO 相关发言：Multigent 知识检索应支持混合召回。"), 0o644); err != nil {
		t.Fatal(err)
	}
	oauthPath := filepath.Join(root, "oauth-notes.md")
	if err := os.WriteFile(oauthPath, []byte("OAuth token 的详细处理说明。"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ds.Add(&DocEntry{
		Title:       "CEO 发言纪要",
		FilePath:    ceoPath,
		Index:       "leadership/ceo",
		CreatedBy:   "human",
		Tags:        []string{"ceo", "strategy"},
		Description: "记录 2026 年 8 月的 CEO 发言",
		CreatedAt:   time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ds.Add(&DocEntry{
		Title:       "OAuth 方案",
		FilePath:    oauthPath,
		Index:       "engineering/backend",
		CreatedBy:   "agent",
		Tags:        []string{"auth"},
		Description: "OAuth token 的处理说明",
		CreatedAt:   time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	results, err := ds.QueryDocs("2026年8月 CEO 相关发言", QueryOptions{
		WithContent: true,
		MaxResults:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Title != "CEO 发言纪要" {
		t.Fatalf("expected CEO doc first, got %q", results[0].Title)
	}
	if results[0].Snippet == "" {
		t.Fatal("expected a snippet in the top result")
	}
	if !contains(results[0].MatchedFields, "title") && !contains(results[0].MatchedFields, "content") {
		t.Fatalf("expected title/content match, got %#v", results[0].MatchedFields)
	}

	mixed, err := ds.QueryDocs("CEO相关发言", QueryOptions{WithContent: true, MaxResults: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(mixed) == 0 || mixed[0].Title != "CEO 发言纪要" {
		t.Fatalf("expected mixed-language query to hit CEO doc, got %#v", mixed)
	}
}

func TestQueryDocsFiltersByScopeAndDate(t *testing.T) {
	root := t.TempDir()
	ds := NewDocsStore(root)

	recentPath := filepath.Join(root, "recent.md")
	oldPath := filepath.Join(root, "old.md")
	if err := os.WriteFile(recentPath, []byte("最近的会议记录"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("老的会议记录"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ds.Add(&DocEntry{
		Title:     "最近会议",
		FilePath:  recentPath,
		Index:     "leadership",
		CreatedBy: "human",
		Tags:      []string{"meeting"},
		CreatedAt: time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ds.Add(&DocEntry{
		Title:     "很早的会议",
		FilePath:  oldPath,
		Index:     "engineering",
		CreatedBy: "agent",
		Tags:      []string{"meeting"},
		CreatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	since := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	results, err := ds.QueryDocs("会议", QueryOptions{
		WithContent: true,
		MaxResults:  10,
		IndexPrefix: "leadership",
		Tag:         "meeting",
		Since:       &since,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].Title != "最近会议" {
		t.Fatalf("expected recent doc, got %q", results[0].Title)
	}
}

func TestWriteContentUpdatesManagedDocument(t *testing.T) {
	root := t.TempDir()
	ds := NewDocsStore(root)
	entry := &DocEntry{Title: "可编辑文章", Index: "drafts", CreatedBy: "human"}
	if err := ds.AddManagedContent(entry, "第一版\n", "article.md"); err != nil {
		t.Fatal(err)
	}

	if err := ds.WriteContent(entry.ID, "第二版\n\n包含更多内容"); err != nil {
		t.Fatal(err)
	}
	got, err := ds.ReadContent(entry.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "第二版\n\n包含更多内容" {
		t.Fatalf("content = %q, want updated content", got)
	}

	if err := ds.WriteContent(entry.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, err = ds.ReadContent(entry.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("content = %q, want empty content", got)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
