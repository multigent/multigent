package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
)

type cliInteractionLease struct {
	db      controldb.Store
	session controldb.InteractionSession
	done    bool
}

func acquireCLIInteraction(root, project, agent, sourceKind, sourceChannel, actorID, reason string) (*cliInteractionLease, bool, error) {
	db, err := openControlDB()
	if err != nil {
		return nil, false, err
	}
	workspaceID, err := workspaceIDForRoot(db, root)
	if err != nil {
		_ = db.Close()
		return nil, false, err
	}
	if workspaceID == "" {
		_ = db.Close()
		return nil, false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := controldb.InteractionSession{
		ID:             newCLIInteractionID("sess"),
		WorkspaceID:    workspaceID,
		ProjectID:      strings.TrimSpace(project),
		AgentID:        strings.TrimSpace(agent),
		SourceKind:     strings.TrimSpace(sourceKind),
		SourceChannel:  strings.TrimSpace(sourceChannel),
		ActorType:      "system",
		ActorID:        strings.TrimSpace(actorID),
		Status:         "active",
		LockReason:     strings.TrimSpace(reason),
		MetadataJSON:   "{}",
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	}
	if session.LockReason == "" {
		session.LockReason = "running_task"
	}
	if err := db.CreateInteractionSession(session); err != nil {
		if active, found, lookupErr := db.ActiveInteractionSession(workspaceID, project, agent); lookupErr == nil && found {
			if shouldRecoverStaleCLIInteraction(active, sourceKind, reason) {
				active.Status = "failed"
				active.UpdatedAt = now
				active.LastActivityAt = now
				active.CompletedAt = now
				_ = db.UpdateInteractionSession(active)
				if retryErr := db.CreateInteractionSession(session); retryErr == nil {
					lease := &cliInteractionLease{db: db, session: session}
					_ = lease.event("system", session.ActorID, session.SourceChannel, "session_acquired", "", map[string]any{
						"sourceKind":    session.SourceKind,
						"sourceChannel": session.SourceChannel,
						"lockReason":    session.LockReason,
						"recoveredFrom": active.ID,
					})
					return lease, false, nil
				}
			}
			_ = db.Close()
			return &cliInteractionLease{session: active}, true, nil
		}
		_ = db.Close()
		return nil, false, err
	}
	lease := &cliInteractionLease{db: db, session: session}
	_ = lease.event("system", session.ActorID, session.SourceChannel, "session_acquired", "", map[string]any{
		"sourceKind":    session.SourceKind,
		"sourceChannel": session.SourceChannel,
		"lockReason":    session.LockReason,
	})
	return lease, false, nil
}

func shouldRecoverStaleCLIInteraction(active controldb.InteractionSession, sourceKind, reason string) bool {
	if strings.TrimSpace(sourceKind) != "scheduler" || strings.TrimSpace(reason) != "running_task" {
		return false
	}
	if strings.TrimSpace(active.SourceKind) != "scheduler" || strings.TrimSpace(active.LockReason) != "running_task" {
		return false
	}
	lastRaw := strings.TrimSpace(active.LastActivityAt)
	if lastRaw == "" {
		lastRaw = strings.TrimSpace(active.UpdatedAt)
	}
	last, err := time.Parse(time.RFC3339, lastRaw)
	if err != nil {
		return false
	}
	return time.Since(last) > 2*time.Minute
}

func (l *cliInteractionLease) Release() {
	if l == nil || l.done {
		return
	}
	l.done = true
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.Status = "completed"
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	l.session.CompletedAt = now
	_ = l.db.UpdateInteractionSession(l.session)
	_ = l.event("system", "", l.session.SourceChannel, "session_released", "", nil)
	_ = l.db.Close()
}

func (l *cliInteractionLease) Fail(reason string) {
	if l == nil || l.done {
		return
	}
	l.done = true
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.Status = "failed"
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	l.session.CompletedAt = now
	_ = l.db.UpdateInteractionSession(l.session)
	_ = l.event("system", "", l.session.SourceChannel, "session_failed", reason, nil)
	_ = l.db.Close()
}

func (l *cliInteractionLease) SetRuntimeSessionID(runtimeSessionID string) {
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if l == nil || l.db == nil || l.done || runtimeSessionID == "" || runtimeSessionID == l.session.RuntimeSessionID {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	l.session.RuntimeSessionID = runtimeSessionID
	l.session.UpdatedAt = now
	l.session.LastActivityAt = now
	_ = l.db.UpdateInteractionSession(l.session)
	_ = l.event("system", "", l.session.SourceChannel, "runtime_session_updated", "", map[string]any{
		"runtimeSessionId": runtimeSessionID,
	})
}

func (l *cliInteractionLease) event(actorType, actorID, channel, eventType, content string, metadata map[string]any) error {
	if l == nil || l.db == nil {
		return nil
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, _ := json.Marshal(metadata)
	return l.db.CreateInteractionEvent(controldb.InteractionEvent{
		ID:           newCLIInteractionID("evt"),
		SessionID:    l.session.ID,
		WorkspaceID:  l.session.WorkspaceID,
		ActorType:    strings.TrimSpace(actorType),
		ActorID:      strings.TrimSpace(actorID),
		Channel:      strings.TrimSpace(channel),
		EventType:    strings.TrimSpace(eventType),
		Content:      content,
		MetadataJSON: string(raw),
	})
}

func workspaceIDForRoot(db controldb.Store, root string) (string, error) {
	workspaces, err := db.ListWorkspaces()
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(root)
	if err != nil {
		target = root
	}
	target = filepath.Clean(target)
	for _, workspace := range workspaces {
		candidate, err := filepath.Abs(workspace.Root)
		if err != nil {
			candidate = workspace.Root
		}
		if filepath.Clean(candidate) == target {
			return workspace.ID, nil
		}
	}
	return "", nil
}

func newCLIInteractionID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
