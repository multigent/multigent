package main

import (
	"fmt"
	"path/filepath"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
)

type cliWorkspaceDB struct {
	db          *controldb.SQLiteStore
	workspace   controldb.Workspace
	workspaceID string
}

func openCLIWorkspaceDB(workspaceRef string) (*cliWorkspaceDB, error) {
	root, rootErr := resolveRoot()
	db, err := controldb.OpenDefault()
	if err != nil {
		return nil, err
	}
	rows, err := db.ListWorkspaces()
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	ref := strings.TrimSpace(workspaceRef)
	if ref != "" {
		for _, row := range rows {
			if workspaceMatchesRef(row, ref) {
				return &cliWorkspaceDB{db: db, workspace: row, workspaceID: row.ID}, nil
			}
		}
		_ = db.Close()
		return nil, fmt.Errorf("workspace %q not found", ref)
	}

	if rootErr != nil {
		_ = db.Close()
		return nil, rootErr
	}
	id, err := workspaceIDForRoot(db, root)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if id == "" {
		_ = db.Close()
		return nil, fmt.Errorf("workspace for root %q not found", root)
	}
	for _, row := range rows {
		if row.ID == id {
			return &cliWorkspaceDB{db: db, workspace: row, workspaceID: row.ID}, nil
		}
	}
	_ = db.Close()
	return nil, fmt.Errorf("workspace %q not found", id)
}

func (c *cliWorkspaceDB) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func workspaceMatchesRef(row controldb.Workspace, ref string) bool {
	rawRef := strings.TrimSpace(ref)
	lowerRef := strings.ToLower(rawRef)
	if rawRef == "" {
		return false
	}
	if strings.ToLower(row.ID) == lowerRef ||
		strings.ToLower(row.Name) == lowerRef ||
		strings.ToLower(row.Slug) == lowerRef {
		return true
	}
	if row.Root == "" {
		return false
	}
	absRoot, err := filepath.Abs(row.Root)
	if err != nil {
		absRoot = row.Root
	}
	absRef, err := filepath.Abs(rawRef)
	if err != nil {
		absRef = rawRef
	}
	return strings.ToLower(filepath.Clean(absRoot)) == strings.ToLower(filepath.Clean(absRef))
}
