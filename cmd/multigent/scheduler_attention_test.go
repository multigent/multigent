package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

func TestPendingAttentionSectionAndSeenMark(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MULTIGENT_CONTROL_DATA_DIR", "")
	t.Setenv("MULTIGENT_DATA_DIR", root)
	if err := os.MkdirAll(filepath.Join(root, ".multigent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".multigent", "agency.yaml"), []byte("name: Test\nlang: zh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := controldb.Open(filepath.Join(root, ".multigent", "multigent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nowTime := time.Now().UTC().Add(-2 * time.Hour)
	now := nowTime.Format(time.RFC3339)
	if err := db.UpsertWorkspace(controldb.Workspace{ID: "ws", Name: "Test", Slug: "test", Root: root, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAgentWorker(controldb.AgentWorker{ID: "aw-pm", WorkspaceID: "ws", Name: "nova", DisplayName: "Nova", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProjectMembership(controldb.ProjectMembership{
		ID:          "pm-1",
		WorkspaceID: "ws",
		ProjectID:   "sample",
		MemberType:  "agent_worker",
		MemberID:    "aw-pm",
		Role:        "pm",
		Title:       "pm",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-one",
		WorkspaceID:   "ws",
		AgentWorkerID: "aw-pm",
		DedupeKey:     "im:lark:om_one",
		SourceKind:    "im_message",
		SourceID:      "om_one",
		SourceChannel: "im:lark:p2p:oc_one:user:glenn",
		Reason:        "im_direct_message",
		Priority:      "normal",
		ActorType:     "user",
		ActorID:       "glenn",
		Summary:       "请看一下当前流程",
		Status:        "pending",
		CreatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAttentionSignal(controldb.AttentionSignal{
		ID:            "sig-two",
		WorkspaceID:   "ws",
		AgentWorkerID: "aw-pm",
		DedupeKey:     "im:lark:om_two",
		SourceKind:    "im_message",
		SourceID:      "om_two",
		SourceChannel: "im:lark:p2p:oc_two:user:glenn",
		Reason:        "im_direct_message",
		Priority:      "normal",
		ActorType:     "user",
		ActorID:       "glenn",
		Summary:       "这条已经 seen 但还没处理",
		Status:        "seen",
		CreatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}

	section, ids, err := pendingAttentionSection(root, "sample", "pm", wakeupStrings("zh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "sig-one" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
	for _, want := range []string{"注意力信号", "sig-one", "Observed:", "约 2 小时前", "im_direct_message", "请看一下当前流程"} {
		if !strings.Contains(section, want) {
			t.Fatalf("section missing %q:\n%s", want, section)
		}
	}
	if strings.Contains(section, "sig-two") || strings.Contains(section, "这条已经 seen 但还没处理") {
		t.Fatalf("seen signal was re-injected:\n%s", section)
	}
	markAttentionSignalsSeen(root, ids)
	signal, ok, err := db.AttentionSignalByID("ws", "sig-one")
	if err != nil || !ok {
		t.Fatalf("lookup signal ok=%v err=%v", ok, err)
	}
	if signal.Status != "seen" || signal.SeenAt == "" {
		t.Fatalf("signal was not marked seen: %#v", signal)
	}
}

func TestSchedulerWakeupTimeSection(t *testing.T) {
	now := time.Date(2026, 8, 23, 10, 30, 0, 0, time.UTC)
	section := schedulerWakeupTimeSection(now, wakeupStrings("zh"))
	for _, want := range []string{"时间上下文", "本次唤醒时间", "2026-08-23T10:30:00Z", "发生时间与距今多久"} {
		if !strings.Contains(section, want) {
			t.Fatalf("time section missing %q:\n%s", want, section)
		}
	}
}
