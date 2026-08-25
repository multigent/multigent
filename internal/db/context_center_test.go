package db

import (
	"path/filepath"
	"testing"
)

func TestContextCenterSourceItemSubscription(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "multigent.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	workspaceID := "ws-context-center"
	if err := store.UpsertWorkspace(Workspace{
		ID:        workspaceID,
		Name:      "Context Center",
		Slug:      "context-center",
		Root:      t.TempDir(),
		CreatedAt: nowUTC(),
	}); err != nil {
		t.Fatalf("upsert workspace: %v", err)
	}

	source := ContextSource{
		ID:           "ctxsrc-lark",
		WorkspaceID:  workspaceID,
		Type:         "lark_im",
		Name:         "TapNow Lark",
		ConfigJSON:   `{"chat":"general"}`,
		MetadataJSON: `{"tenant":"tapnow"}`,
		CreatedBy:    "admin",
	}
	if err := store.UpsertContextSource(source); err != nil {
		t.Fatalf("upsert source: %v", err)
	}
	sources, err := store.ListContextSources(ContextSourceFilter{WorkspaceID: workspaceID, Type: "lark_im"})
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 || sources[0].ID != source.ID || sources[0].Status != "active" {
		t.Fatalf("unexpected sources: %+v", sources)
	}

	item := ContextItem{
		ID:          "ctx-1",
		WorkspaceID: workspaceID,
		SourceID:    source.ID,
		SourceType:  "lark_im",
		ProjectID:   "tapnow-mcp-server",
		Title:       "OAuth 联调讨论",
		Summary:     "Joey 反馈 JWKS 配置需要确认。",
		ContentText: "群聊原始消息内容",
		DedupeKey:   "lark:msg-1",
		LabelsJSON:  `{"topic":"oauth"}`,
	}
	if err := store.UpsertContextItem(item); err != nil {
		t.Fatalf("upsert item: %v", err)
	}
	item.ID = "ctx-duplicate"
	item.Title = "OAuth 联调讨论更新"
	if err := store.UpsertContextItem(item); err != nil {
		t.Fatalf("upsert duplicate item: %v", err)
	}
	items, err := store.ListContextItems(ContextItemFilter{WorkspaceID: workspaceID, SourceType: "lark_im", Query: "JWKS"})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 || items[0].Title != "OAuth 联调讨论更新" {
		t.Fatalf("unexpected deduped items: %+v", items)
	}
	got, ok, err := store.ContextItemByID(workspaceID, items[0].ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if !ok || got.ContentText != "群聊原始消息内容" {
		t.Fatalf("unexpected item: ok=%v item=%+v", ok, got)
	}
	got, ok, err = store.ContextItemByID(workspaceID, items[0].ID)
	if err != nil || !ok {
		t.Fatalf("get item second time: ok=%v err=%v", ok, err)
	}
	if got.UsageCount != 1 {
		t.Fatalf("usage count should reflect previous read, got %+v", got)
	}

	sub := ContextSubscription{
		ID:             "ctxsub-mason",
		WorkspaceID:    workspaceID,
		SubscriberType: "agent_worker",
		SubscriberID:   "aw-mason",
		SourceIDsJSON:  `["ctxsrc-lark"]`,
		SignalRuleJSON: `{"mention":true}`,
	}
	if err := store.UpsertContextSubscription(sub); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}
	subs, err := store.ListContextSubscriptions(ContextSubscriptionFilter{WorkspaceID: workspaceID, SubscriberType: "agent_worker"})
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].SubscriberID != "aw-mason" || subs[0].DeliveryMode != "searchable" {
		t.Fatalf("unexpected subscriptions: %+v", subs)
	}
}
