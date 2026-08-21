package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/multigent/multigent/internal/agentdir"
	controldb "github.com/multigent/multigent/internal/db"
)

type cliProjectWorker struct {
	Worker     controldb.AgentWorker
	Membership controldb.ProjectMembership
	Name       string
}

func listCLIProjectWorkers(root, project string) ([]cliProjectWorker, error) {
	db, err := openControlDBForRoot(root)
	if err != nil {
		return nil, err
	}
	workspaceID, err := workspaceIDForRoot(db, root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace is not registered in control DB")
	}
	resolved, err := agentdir.New(db).ProjectWorkers(workspaceID, project)
	if err != nil {
		return nil, err
	}
	out := make([]cliProjectWorker, 0, len(resolved))
	for _, item := range resolved {
		name := strings.TrimSpace(item.Membership.Title)
		if name == "" {
			name = strings.TrimSpace(item.Worker.Name)
		}
		if name == "" {
			name = strings.TrimSpace(item.Worker.ID)
		}
		out = append(out, cliProjectWorker{Worker: item.Worker, Membership: item.Membership, Name: name})
	}
	return out, nil
}

func listCLIProjectAgentNames(root, project string) ([]string, error) {
	workers, err := listCLIProjectWorkers(root, project)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(workers))
	for _, worker := range workers {
		names = append(names, worker.Name)
	}
	return names, nil
}

func cliProjectAgentDir(root, project, agent string) string {
	return filepath.Join(root, "projects", project, "agents", agent)
}
