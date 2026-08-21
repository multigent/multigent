package db

import (
	"database/sql"
	"errors"
	"strings"
)

func (db *SQLiteStore) UpsertProjectMembership(m ProjectMembership) error {
	if m.CreatedAt == "" {
		m.CreatedAt = nowUTC()
	}
	if m.UpdatedAt == "" {
		m.UpdatedAt = nowUTC()
	}
	if m.PermissionsJSON == "" {
		m.PermissionsJSON = "[]"
	}
	if m.PriorityWeight == 0 {
		m.PriorityWeight = 1.0
	}
	_, err := db.sql.Exec(`INSERT INTO project_memberships (
	id, workspace_id, project_id, member_type, member_id, role, title, prompt,
	permissions_json, auto_pick_tasks, attention_enabled, priority_weight,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, project_id, member_type, member_id) DO UPDATE SET
	role = excluded.role,
	title = excluded.title,
	prompt = excluded.prompt,
	permissions_json = excluded.permissions_json,
	auto_pick_tasks = excluded.auto_pick_tasks,
	attention_enabled = excluded.attention_enabled,
	priority_weight = excluded.priority_weight,
	updated_at = excluded.updated_at`,
		m.ID, m.WorkspaceID, m.ProjectID, m.MemberType, m.MemberID, m.Role, m.Title, m.Prompt,
		m.PermissionsJSON, boolInt(m.AutoPickTasks), boolInt(m.AttentionEnabled), m.PriorityWeight,
		m.CreatedAt, m.UpdatedAt)
	return err
}

func (db *SQLiteStore) ProjectMembershipByID(workspaceID, id string) (ProjectMembership, bool, error) {
	row := db.sql.QueryRow(`SELECT id, workspace_id, project_id, member_type, member_id, role, title, prompt,
permissions_json, auto_pick_tasks, attention_enabled, priority_weight, created_at, updated_at
FROM project_memberships WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	return scanProjectMembershipFound(row)
}

func (db *SQLiteStore) ListProjectMemberships(filter ProjectMembershipFilter) ([]ProjectMembership, error) {
	query := `SELECT id, workspace_id, project_id, member_type, member_id, role, title, prompt,
permissions_json, auto_pick_tasks, attention_enabled, priority_weight, created_at, updated_at
FROM project_memberships WHERE 1=1`
	args := make([]any, 0, 4)
	if strings.TrimSpace(filter.WorkspaceID) != "" {
		query += ` AND workspace_id = ?`
		args = append(args, strings.TrimSpace(filter.WorkspaceID))
	}
	if strings.TrimSpace(filter.ProjectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if strings.TrimSpace(filter.MemberType) != "" {
		query += ` AND member_type = ?`
		args = append(args, strings.TrimSpace(filter.MemberType))
	}
	if strings.TrimSpace(filter.MemberID) != "" {
		query += ` AND member_id = ?`
		args = append(args, strings.TrimSpace(filter.MemberID))
	}
	query += ` ORDER BY project_id ASC, member_type ASC, role ASC, member_id ASC`
	rows, err := db.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectMembership, 0)
	for rows.Next() {
		m, err := scanProjectMembership(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (db *SQLiteStore) DeleteProjectMembership(workspaceID, id string) error {
	_, err := db.sql.Exec(`DELETE FROM project_memberships WHERE workspace_id = ? AND id = ?`, strings.TrimSpace(workspaceID), strings.TrimSpace(id))
	return err
}

type projectMembershipScanner interface {
	Scan(dest ...any) error
}

func scanProjectMembershipFound(row projectMembershipScanner) (ProjectMembership, bool, error) {
	m, err := scanProjectMembership(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectMembership{}, false, nil
	}
	if err != nil {
		return ProjectMembership{}, false, err
	}
	return m, true, nil
}

func scanProjectMembership(row projectMembershipScanner) (ProjectMembership, error) {
	var m ProjectMembership
	var autoPick, attention int
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.ProjectID, &m.MemberType, &m.MemberID, &m.Role, &m.Title, &m.Prompt,
		&m.PermissionsJSON, &autoPick, &attention, &m.PriorityWeight, &m.CreatedAt, &m.UpdatedAt)
	m.AutoPickTasks = autoPick != 0
	m.AttentionEnabled = attention != 0
	return m, err
}
