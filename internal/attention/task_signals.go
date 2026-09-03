// Package attention contains shared lifecycle helpers for attention signals.
package attention

import (
	"encoding/json"
	"fmt"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

const signalIDsVar = "MULTIGENT_ATTENTION_SIGNAL_IDS_JSON"

// SignalIDsForTask returns the attention signals attached to a wakeup task.
// Tasks without this explicit scheduler metadata are never treated as signal
// consumers, even when their prompt happens to mention an attention signal.
func SignalIDsForTask(task *entity.Task) []string {
	if task == nil || strings.TrimSpace(string(task.Type)) != "wakeup" {
		return nil
	}
	raw := strings.TrimSpace(task.Vars[signalIDsVar])
	if raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// CloseTaskSignals marks every signal attached to a completed wakeup task as
// handled and records the consuming task/run. The database transition is
// idempotent and never reopens an already terminal signal.
func CloseTaskSignals(db controldb.Store, workspaceID string, task *entity.Task, resultRef string) error {
	if db == nil || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	ids := SignalIDsForTask(task)
	for _, id := range ids {
		if err := db.MarkAttentionSignalStatusWithResult(workspaceID, id, "handled", resultRef); err != nil {
			return fmt.Errorf("mark attention signal %s handled: %w", id, err)
		}
	}
	return nil
}
