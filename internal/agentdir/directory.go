// Package agentdir resolves the 2.x Agent Worker identity model.
package agentdir

import (
	"fmt"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
)

const (
	MemberTypeAgentWorker = "agent_worker"
	MemberTypeUser        = "user"
)

type Store interface {
	AgentWorkerByID(workspaceID, id string) (controldb.AgentWorker, bool, error)
	AgentWorkerByName(workspaceID, name string) (controldb.AgentWorker, bool, error)
	ListAgentWorkers(workspaceID string) ([]controldb.AgentWorker, error)
	ProjectMembershipByID(workspaceID, id string) (controldb.ProjectMembership, bool, error)
	ListProjectMemberships(filter controldb.ProjectMembershipFilter) ([]controldb.ProjectMembership, error)
}

type Directory struct {
	db Store
}

type ProjectWorker struct {
	Worker     controldb.AgentWorker
	Membership controldb.ProjectMembership
}

func New(db Store) *Directory {
	return &Directory{db: db}
}

func (d *Directory) Worker(workspaceID, ref string) (controldb.AgentWorker, bool, error) {
	if d == nil || d.db == nil {
		return controldb.AgentWorker{}, false, fmt.Errorf("agent directory store is nil")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	ref = strings.TrimSpace(ref)
	if workspaceID == "" || ref == "" {
		return controldb.AgentWorker{}, false, nil
	}
	if worker, ok, err := d.db.AgentWorkerByID(workspaceID, ref); err != nil {
		return controldb.AgentWorker{}, false, err
	} else if ok {
		return worker, true, nil
	}
	return d.db.AgentWorkerByName(workspaceID, ref)
}

func (d *Directory) ProjectWorkerByMembership(workspaceID, membershipID string) (ProjectWorker, bool, error) {
	if d == nil || d.db == nil {
		return ProjectWorker{}, false, fmt.Errorf("agent directory store is nil")
	}
	membership, ok, err := d.db.ProjectMembershipByID(strings.TrimSpace(workspaceID), strings.TrimSpace(membershipID))
	if err != nil || !ok {
		return ProjectWorker{}, ok, err
	}
	if membership.MemberType != MemberTypeAgentWorker {
		return ProjectWorker{}, false, nil
	}
	worker, ok, err := d.db.AgentWorkerByID(workspaceID, membership.MemberID)
	if err != nil || !ok {
		return ProjectWorker{}, ok, err
	}
	return ProjectWorker{Worker: worker, Membership: membership}, true, nil
}

func (d *Directory) ProjectWorker(workspaceID, projectID, workerRef string) (ProjectWorker, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	projectID = strings.TrimSpace(projectID)
	workerRef = strings.TrimSpace(workerRef)
	if workspaceID == "" || projectID == "" || workerRef == "" {
		return ProjectWorker{}, false, nil
	}
	memberships, err := d.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		MemberType:  MemberTypeAgentWorker,
	})
	if err != nil {
		return ProjectWorker{}, false, err
	}
	for _, membership := range memberships {
		if sameIdentity(membership.ID, workerRef) || sameIdentity(membership.MemberID, workerRef) || sameIdentity(membership.Title, workerRef) {
			worker, ok, err := d.db.AgentWorkerByID(workspaceID, membership.MemberID)
			if err != nil || !ok {
				return ProjectWorker{}, ok, err
			}
			return ProjectWorker{Worker: worker, Membership: membership}, true, nil
		}
		worker, ok, err := d.db.AgentWorkerByID(workspaceID, membership.MemberID)
		if err != nil {
			return ProjectWorker{}, false, err
		}
		if !ok {
			continue
		}
		if sameIdentity(worker.ID, workerRef) || sameIdentity(worker.Name, workerRef) || sameIdentity(worker.DisplayName, workerRef) {
			return ProjectWorker{Worker: worker, Membership: membership}, true, nil
		}
	}
	return ProjectWorker{}, false, nil
}

func (d *Directory) ProjectWorkers(workspaceID, projectID string) ([]ProjectWorker, error) {
	memberships, err := d.db.ListProjectMemberships(controldb.ProjectMembershipFilter{
		WorkspaceID: strings.TrimSpace(workspaceID),
		ProjectID:   strings.TrimSpace(projectID),
		MemberType:  MemberTypeAgentWorker,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectWorker, 0, len(memberships))
	for _, membership := range memberships {
		worker, ok, err := d.db.AgentWorkerByID(workspaceID, membership.MemberID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, ProjectWorker{Worker: worker, Membership: membership})
	}
	return out, nil
}

func (d *Directory) ResolveLegacyMailbox(workspaceID, mailbox string) (ProjectWorker, bool, error) {
	projectID, legacyAgent, ok := SplitLegacyMailbox(mailbox)
	if !ok {
		return ProjectWorker{}, false, nil
	}
	workers, err := d.ProjectWorkers(workspaceID, projectID)
	if err != nil {
		return ProjectWorker{}, false, err
	}
	for _, candidate := range workers {
		if sameIdentity(candidate.Membership.Title, legacyAgent) ||
			sameIdentity(candidate.Worker.Name, legacyAgent) ||
			sameIdentity(candidate.Worker.DisplayName, legacyAgent) {
			return candidate, true, nil
		}
	}
	return ProjectWorker{}, false, nil
}

func SplitLegacyMailbox(mailbox string) (projectID, agentName string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(mailbox), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	projectID = strings.TrimSpace(parts[0])
	agentName = strings.TrimSpace(parts[1])
	return projectID, agentName, projectID != "" && agentName != ""
}

func sameIdentity(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
