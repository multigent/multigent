package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/scaffold"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const (
	exampleWorkspaceName = "Example Collaboration Lab"
	exampleProjectName   = "hello-world-relay"
	exampleTeamName      = "collaboration-demo"
	exampleWorkflowID    = "wf-example-hello-world-relay"
)

func (s *Server) handleCreateExampleWorkspace(w http.ResponseWriter, r *http.Request) {
	cur := s.currentUser(r)
	if cur == nil || cur.Username == "" || cur.Username == "apikey" {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeAuthenticatedUserRequired, "authenticated user required")
		return
	}
	if s.controlDB == nil {
		s.jsonErrorCode(w, http.StatusServiceUnavailable, ErrCodeWorkspaceDatabaseUnavailable, "control database unavailable")
		return
	}

	id := newWorkspaceID()
	absRoot, err := filepath.Abs(filepath.Join(defaultWorkspaceDataDir(), id))
	if err != nil {
		s.jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		s.serverError(w, err)
		return
	}

	now := time.Now().UTC()
	agency := &entity.Agency{
		Name:        exampleWorkspaceName,
		Description: "A built-in learning workspace that demonstrates agent handoff, human review, structured workflow output, and shared docs without assuming a specific industry.",
		CreatedBy:   cur.Username,
		CreatedAt:   now.Format(time.RFC3339),
	}
	if err := scaffold.InitAgency(absRoot, agency); err != nil {
		s.serverError(w, err)
		return
	}

	ref := workspaceRef{
		ID:          id,
		Name:        agency.Name,
		Description: agency.Description,
		Root:        absRoot,
		CreatedBy:   agency.CreatedBy,
		CreatedAt:   agency.CreatedAt,
	}
	if err := s.upsertWorkspaceRef(ref); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.controlDB.UpsertWorkspaceMember(ref.ID, cur.Username, WorkspaceRoleOwner); err != nil {
		s.serverError(w, err)
		return
	}

	exampleStore := store.NewDB(absRoot, s.controlDB)
	exampleTasks := taskstore.NewDB(absRoot, s.controlDB)
	if err := seedExampleWorkspace(absRoot, ref.ID, cur.Username, exampleStore, exampleTasks, s.controlDB); err != nil {
		s.serverError(w, err)
		return
	}

	if err := s.switchWorkspaceRoot(absRoot); err != nil {
		s.serverError(w, err)
		return
	}
	ref.Active = true
	s.auditLog(auditLogInput{
		WorkspaceID:  ref.ID,
		Action:       "workspace.example.create",
		ResourceType: "workspace",
		ResourceID:   ref.ID,
		Summary:      "Example workspace created",
		After: map[string]any{
			"id":        ref.ID,
			"name":      ref.Name,
			"project":   exampleProjectName,
			"workflow":  exampleWorkflowID,
			"createdBy": cur.Username,
		},
		Request: r,
	})
	_ = json.NewEncoder(w).Encode(ref)
}

func seedExampleWorkspace(root, workspaceID, username string, st store.Store, ts taskstore.Store, db controldb.Store) error {
	if err := st.SaveAgencyPrompt(exampleAgencyPrompt()); err != nil {
		return fmt.Errorf("save agency prompt: %w", err)
	}
	if err := st.SaveTeam(exampleTeamName, &entity.Team{
		Name:        exampleTeamName,
		Description: "A neutral demo team for practicing agent-to-agent relay and human review.",
		Owners:      []string{username},
		Goals: []string{
			"Show how work moves through multiple agents.",
			"Keep human intervention explicit and lightweight.",
			"Store durable outputs in the workspace docs.",
		},
	}); err != nil {
		return fmt.Errorf("save example team: %w", err)
	}
	if err := st.SaveTeamPrompt(exampleTeamName, exampleTeamPrompt()); err != nil {
		return fmt.Errorf("save example team prompt: %w", err)
	}
	for _, role := range exampleRoles(username) {
		if err := st.SaveRole(exampleTeamName, role.Name, role.Role); err != nil {
			return fmt.Errorf("save example role %s: %w", role.Name, err)
		}
		if err := st.SaveRolePrompt(exampleTeamName, role.Name, role.Prompt); err != nil {
			return fmt.Errorf("save example role prompt %s: %w", role.Name, err)
		}
	}
	if err := st.SaveProject(exampleProjectName, &entity.Project{
		Name:        exampleProjectName,
		Description: "A Hello World relay project that proves Multigent can route one task across agents and humans.",
		Owners:      []string{username},
	}); err != nil {
		return fmt.Errorf("save example project: %w", err)
	}
	if err := st.SaveProjectPrompt(exampleProjectName, exampleProjectPrompt()); err != nil {
		return fmt.Errorf("save example project prompt: %w", err)
	}
	for _, agent := range exampleAgents(username) {
		if err := os.MkdirAll(st.AgentDir(exampleProjectName, agent.Name), 0o755); err != nil {
			return fmt.Errorf("create example agent dir %s: %w", agent.Name, err)
		}
		if err := st.SaveAgentMeta(exampleProjectName, agent.Name, agent); err != nil {
			return fmt.Errorf("save example agent %s: %w", agent.Name, err)
		}
	}
	if err := seedExampleDocs(root, username); err != nil {
		return fmt.Errorf("seed example docs: %w", err)
	}
	wfStore := workflowstore.NewStore(db, workspaceID)
	def := exampleWorkflowDefinition()
	if err := wfStore.SaveDefinition(&def); err != nil {
		return fmt.Errorf("save example workflow definition: %w", err)
	}
	task := exampleTask(username)
	if err := ts.AddTask(exampleProjectName, "greeter-agent", task); err != nil {
		return fmt.Errorf("add example task: %w", err)
	}
	_, _, err := wfStore.StartRun(exampleProjectName, task.ID, def.ID, map[string]entity.WorkflowActorBinding{
		"greeter":        {Type: "agent", ID: "greeter-agent"},
		"greetingReview": {Type: "human", ID: username},
		"responder":      {Type: "agent", ID: "responder-agent"},
		"recorder":       {Type: "agent", ID: "recorder-agent"},
		"finalReview":    {Type: "human", ID: username},
	})
	if err != nil {
		return fmt.Errorf("start example workflow run: %w", err)
	}
	return nil
}

type exampleRoleSeed struct {
	Name   string
	Role   *entity.Role
	Prompt string
}

func exampleRoles(owner string) []exampleRoleSeed {
	return []exampleRoleSeed{
		{
			Name: "greeter",
			Role: &entity.Role{Name: "greeter", Description: "Starts a clear, friendly relay and prepares the first handoff.", Owners: []string{owner}},
			Prompt: `You start neutral collaboration relays.

Focus on clarity:
- State the purpose of the relay in one short document.
- Create durable docs for any non-trivial output.
- Hand off with enough context for the next agent to continue without asking the human to repeat themselves.
- Keep the tone simple and universal. Do not assume the workspace is for software, sales, marketing, or any single department.`,
		},
		{
			Name: "responder",
			Role: &entity.Role{Name: "responder", Description: "Reads the upstream handoff and responds with the next useful contribution.", Owners: []string{owner}},
			Prompt: `You continue collaboration relays.

Focus on continuity:
- Read the previous step output before acting.
- Preserve the original intent.
- Add one useful response and a clean handoff for the recorder.
- If anything is ambiguous, make the uncertainty explicit instead of inventing context.`,
		},
		{
			Name: "recorder",
			Role: &entity.Role{Name: "recorder", Description: "Turns the relay into a concise collaboration record.", Owners: []string{owner}},
			Prompt: `You record collaboration outcomes.

Focus on traceability:
- Summarize what each participant contributed.
- Store final notes in docs and return doc IDs in workflow outputs.
- Point out where a human intervened and whether the intervention could be reduced next time.`,
		},
	}
}

func exampleAgents(owner string) []*entity.AgentMeta {
	now := time.Now().UTC()
	return []*entity.AgentMeta{
		exampleAgent("greeter-agent", "greeter", owner, now),
		exampleAgent("responder-agent", "responder", owner, now),
		exampleAgent("recorder-agent", "recorder", owner, now),
	}
}

func exampleAgent(name, role, owner string, now time.Time) *entity.AgentMeta {
	return &entity.AgentMeta{
		Name:          name,
		Project:       exampleProjectName,
		Team:          exampleTeamName,
		Role:          role,
		Model:         entity.ModelClaudeCode,
		HiredAt:       now,
		Owners:        []string{owner},
		RuntimeMode:   "cloud",
		AutonomyLevel: "L1",
		Sandbox: &entity.SandboxConfig{
			Provider: entity.SandboxDocker,
			Docker: &entity.DockerSandboxConfig{
				NetworkMode: "bridge",
			},
		},
	}
}

func seedExampleDocs(root, username string) error {
	ds := store.NewDocsStore(root)
	return ds.AddManagedContent(&store.DocEntry{
		Title:       "Hello World Relay Guide",
		Index:       "examples/hello-world-relay",
		CreatedBy:   username,
		Tags:        []string{"example", "tour"},
		Description: "How to run the built-in Hello World collaboration relay.",
	}, `# Hello World Relay Guide

This workspace is intentionally neutral. It exists to prove a simple loop:

1. One agent starts the work.
2. A human reviews or sends it back.
3. Another agent continues from structured upstream output.
4. A final agent records what happened.
5. The human confirms the result.

Before running the demo, configure at least one model account and attach it to the three demo agents. Then open the seeded task in the project task list and wake the first agent.
`, "hello-world-relay-guide.md")
}

func exampleWorkflowDefinition() entity.WorkflowDefinition {
	field := func(name, desc string) entity.WorkflowField {
		return entity.WorkflowField{Name: name, Description: desc}
	}
	step := func(id, typ, title, desc, role, color string, x, y int, inputs, outputs []entity.WorkflowField) entity.WorkflowStep {
		return entity.WorkflowStep{
			ID:           id,
			Type:         typ,
			Title:        title,
			Description:  desc,
			ActorRole:    role,
			InputFields:  inputs,
			OutputFields: outputs,
			Position:     entity.WorkflowPosition{X: x, Y: y},
			Config:       map[string]string{"color": color},
		}
	}
	edge := func(id, from, to, label string, condition *entity.WorkflowEdgeCondition, mapping map[string]string, def bool) entity.WorkflowEdge {
		return entity.WorkflowEdge{ID: id, From: from, To: to, Label: label, Condition: condition, InputMapping: mapping, IsDefault: def}
	}
	cond := func(field, value string) *entity.WorkflowEdgeCondition {
		return &entity.WorkflowEdgeCondition{Field: field, Operator: "eq", Value: value}
	}

	return entity.WorkflowDefinition{
		ID:          exampleWorkflowID,
		Name:        "Hello World Collaboration Relay",
		Description: "A minimal workflow that demonstrates agent handoff, human review loops, structured outputs, and document references without assuming a business domain.",
		Version:     1,
		Scope:       "workspace",
		StartStepID: "greeting",
		Steps: []entity.WorkflowStep{
			step("greeting", "agent_task", "Start Greeting", "Create the first greeting and a handoff note. Longer content must be stored as docs and returned as doc IDs.", "greeter", "sky", 80, 120, nil, []entity.WorkflowField{
				field("greeting_doc_id", "Doc ID containing the greeting and purpose of the relay."),
				field("handoff_note_doc_id", "Doc ID containing the handoff note for the next actor."),
				field("summary", "One-sentence summary of this step."),
			}),
			step("greeting_review", "human_review", "Review Greeting", "Review whether the greeting is clear enough. Approve it or request changes with concrete comments.", "greetingReview", "amber", 460, 120, []entity.WorkflowField{
				field("greeting_doc_id", "Greeting document from the previous step."),
				field("handoff_note_doc_id", "Handoff document from the previous step."),
			}, []entity.WorkflowField{
				field("decision", "approve or request_changes."),
				field("comments", "Review comments. Required even when approving."),
			}),
			step("response", "agent_task", "Continue Relay", "Read the approved greeting and add the next response with a new handoff.", "responder", "emerald", 840, 120, []entity.WorkflowField{
				field("greeting_doc_id", "Approved greeting document."),
				field("handoff_note_doc_id", "Approved handoff document."),
				field("review_comments", "Human review comments from the approval step."),
			}, []entity.WorkflowField{
				field("response_doc_id", "Doc ID containing the responder contribution."),
				field("next_handoff_doc_id", "Doc ID containing the handoff for the recorder."),
				field("summary", "One-sentence summary of this step."),
			}),
			step("record", "agent_task", "Record Collaboration", "Create a concise record of the whole relay and lessons learned.", "recorder", "violet", 1220, 120, []entity.WorkflowField{
				field("response_doc_id", "Responder contribution document."),
				field("next_handoff_doc_id", "Recorder handoff document."),
			}, []entity.WorkflowField{
				field("collaboration_record_doc_id", "Doc ID containing the final collaboration record."),
				field("learnings_doc_id", "Doc ID containing lessons learned and possible process improvements."),
				field("summary", "One-sentence summary of this step."),
			}),
			step("final_review", "human_review", "Final Review", "Confirm whether the relay demonstrates the expected collaboration loop.", "finalReview", "slate", 1600, 120, []entity.WorkflowField{
				field("collaboration_record_doc_id", "Final collaboration record document."),
				field("learnings_doc_id", "Lessons learned document."),
			}, []entity.WorkflowField{
				field("decision", "approve or request_changes."),
				field("comments", "Final review comments. Required even when approving."),
			}),
		},
		Edges: []entity.WorkflowEdge{
			edge("e-greeting-review", "greeting", "greeting_review", "review", nil, map[string]string{
				"greeting_doc_id":     "$output.greeting_doc_id",
				"handoff_note_doc_id": "$output.handoff_note_doc_id",
			}, true),
			edge("e-review-approve", "greeting_review", "response", "approved", cond("decision", "approve"), map[string]string{
				"greeting_doc_id":     "$input.greeting_doc_id",
				"handoff_note_doc_id": "$input.handoff_note_doc_id",
				"review_comments":     "$output.comments",
			}, false),
			edge("e-review-rework", "greeting_review", "greeting", "changes requested", cond("decision", "request_changes"), map[string]string{
				"review_comments": "$output.comments",
			}, false),
			edge("e-response-record", "response", "record", "record", nil, map[string]string{
				"response_doc_id":     "$output.response_doc_id",
				"next_handoff_doc_id": "$output.next_handoff_doc_id",
			}, true),
			edge("e-record-final", "record", "final_review", "final review", nil, map[string]string{
				"collaboration_record_doc_id": "$output.collaboration_record_doc_id",
				"learnings_doc_id":            "$output.learnings_doc_id",
			}, true),
			edge("e-final-rework", "final_review", "record", "changes requested", cond("decision", "request_changes"), map[string]string{
				"review_comments": "$output.comments",
			}, false),
		},
	}
}

func exampleTask(username string) *entity.Task {
	now := time.Now().UTC()
	return &entity.Task{
		ID:          entity.NewTaskID(),
		Title:       "Complete a Hello World collaboration relay",
		Type:        entity.TaskTypeChore,
		Priority:    2,
		Assignee:    exampleProjectName + "/greeter-agent",
		CreatedBy:   username,
		Status:      entity.TaskStatusPending,
		Description: "Use the built-in workflow to pass one small piece of work from one agent to another, with human review in the middle.",
		Prompt: `Run the Hello World collaboration relay.

Keep the output neutral and easy to inspect. Use docs for the required document outputs, then finish the current workflow step with structured output fields exactly as specified by the workflow context.`,
		Labels:    []string{"example", "tour"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func exampleAgencyPrompt() string {
	return `# Example Collaboration Lab

This workspace is a neutral Multigent demo.

Rules:
- Keep outputs short, inspectable, and durable.
- Use workspace docs for non-trivial artifacts and return doc IDs in workflow outputs.
- Follow the active workflow step exactly.
- Humans review and coach; agents do the repeatable work.`
}

func exampleTeamPrompt() string {
	return `# Collaboration Demo Team

You demonstrate coordination, not a vertical business process.

Every agent should:
- Read upstream workflow inputs before acting.
- Produce structured workflow outputs.
- Avoid assuming a software, sales, marketing, design, or operations scenario.
- Make handoffs clear enough that the next actor does not need the human to repeat context.`
}

func exampleProjectPrompt() string {
	return `# hello-world-relay

This project exists only to demonstrate Multigent coordination.

The goal is to complete one simple relay:
1. Start with a greeting.
2. Let a human approve or request changes.
3. Continue the relay.
4. Record what happened.
5. Let the human make a final decision.`
}
