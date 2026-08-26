package entity

import (
	"fmt"
	"strings"
	"time"
)

// ApplyStatusTimestamps sets StartedAt / FinishedAt based on status transitions.
// StartedAt is set once when entering in_progress; FinishedAt when entering a terminal state.
func ApplyStatusTimestamps(t *Task, prevStatus TaskStatus, now time.Time) {
	if t == nil {
		return
	}
	if t.Status == TaskStatusInProgress && t.StartedAt == nil {
		t.StartedAt = &now
	}
	if t.Status.IsTerminal() && t.FinishedAt == nil {
		t.FinishedAt = &now
	}
	// Re-open to pending (retry): clear execution timestamps.
	if t.Status == TaskStatusPending && prevStatus.IsTerminal() {
		t.StartedAt = nil
		t.FinishedAt = nil
	}
}

// NormalizeEstimateDuration validates and normalizes a Go duration string (e.g. "30m", "2h").
// Empty string clears the estimate.
func NormalizeEstimateDuration(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return "", fmt.Errorf("invalid estimate duration %q: use Go duration syntax (e.g. 30m, 2h)", raw)
	}
	if d <= 0 {
		return "", fmt.Errorf("estimate duration must be positive")
	}
	return d.String(), nil
}

// ParseTaskNotBefore parses the execution gate for a task.
//
// Supported inputs:
//   - RFC3339/RFC3339Nano timestamps, e.g. 2026-08-26T15:30:00+08:00
//   - Local datetime, e.g. 2026-08-26 15:30 or 2026-08-26 15:30:00
//   - Go duration from now, e.g. 10m, 2h
//
// Empty input clears the gate.
func ParseTaskNotBefore(raw string, now time.Time) (*time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d <= 0 {
			return nil, fmt.Errorf("not-before duration must be positive")
		}
		t := now.Add(d).UTC()
		return &t, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	}
	loc := time.Local
	for _, layout := range layouts {
		var (
			t   time.Time
			err error
		)
		if strings.Contains(layout, "Z07") {
			t, err = time.Parse(layout, s)
		} else {
			t, err = time.ParseInLocation(layout, s, loc)
		}
		if err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}
	return nil, fmt.Errorf("invalid not-before %q: use RFC3339, 'YYYY-MM-DD HH:MM', or a duration like 10m", raw)
}

// TaskReady reports whether a task may be executed at now.
func TaskReady(t *Task, now time.Time) bool {
	if t == nil || t.NotBefore == nil {
		return true
	}
	return !t.NotBefore.After(now)
}

// TaskElapsed returns wall-clock elapsed time for a task, or zero if not started.
func TaskElapsed(t *Task, now time.Time) time.Duration {
	if t == nil || t.StartedAt == nil {
		return 0
	}
	end := now
	if t.FinishedAt != nil {
		end = *t.FinishedAt
	}
	if end.Before(*t.StartedAt) {
		return 0
	}
	return end.Sub(*t.StartedAt)
}
