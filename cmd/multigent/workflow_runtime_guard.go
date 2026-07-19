package main

import (
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/runner"
	workflowstore "github.com/multigent/multigent/internal/workflow"
)

const workflowStepNotCompletedError = "workflow step was not completed by the agent; use `mga step done --task-id <id>` with every required output field"

func enforceWorkflowStepCompletion(root, project string, task *entity.Task, result *runner.RunResult) {
	if task == nil || result == nil || !taskStillInProgress(task) || !taskHasActiveWorkflow(root, project, task.ID) {
		return
	}
	result.Status = entity.TaskStatusDoneFailed
	result.ErrorMsg = workflowStepNotCompletedError
}

func taskStillInProgress(task *entity.Task) bool {
	return task != nil && task.Status == entity.TaskStatusInProgress
}

func taskHasActiveWorkflow(root, project, taskID string) bool {
	if strings.TrimSpace(taskID) == "" {
		return false
	}
	db, err := controldb.OpenDefault()
	if err != nil {
		return false
	}
	defer db.Close()
	workspaceID, err := workspaceIDForRoot(db, root)
	if err != nil || strings.TrimSpace(workspaceID) == "" {
		return false
	}
	run, ok, err := workflowstore.NewStore(db, workspaceID).RunForTask(project, taskID)
	if err != nil || !ok {
		return false
	}
	return strings.TrimSpace(run.Status) != "completed"
}
