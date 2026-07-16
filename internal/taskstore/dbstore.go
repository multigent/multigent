package taskstore

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
)

type DBStore struct {
	root        string
	workspaceID string
	db          controldb.Store
	files       Store
}

func NewDB(root string, db controldb.Store) Store {
	workspaceID, _ := ensureWorkspace(root, db)
	return &DBStore{root: root, workspaceID: workspaceID, db: db, files: New(root)}
}

func (s *DBStore) AddTask(project, agent string, t *entity.Task) error {
	if t.IdempotencyKey != "" {
		active, err := s.ListTasks(project, agent)
		if err != nil {
			return err
		}
		for _, existing := range active {
			if existing.IdempotencyKey == t.IdempotencyKey {
				t.ID = existing.ID
				return errs.Conflict("task", t.IdempotencyKey)
			}
		}
	}
	return s.putJSON("tasks", []string{project, agent, t.ID}, t)
}

func (s *DBStore) GetTask(project, agent, id string) (*entity.Task, error) {
	var t entity.Task
	if ok, err := s.getJSON("tasks", []string{project, agent, id}, &t); err != nil {
		return nil, err
	} else if !ok {
		return nil, errs.NotFound("task", id)
	}
	return &t, nil
}

func (s *DBStore) UpdateTask(project, agent string, t *entity.Task) error {
	if _, err := s.GetTask(project, agent, t.ID); err != nil {
		return err
	}
	t.UpdatedAt = time.Now().UTC()
	return s.putJSON("tasks", []string{project, agent, t.ID}, t)
}

func (s *DBStore) PersistTask(project, agent string, t *entity.Task) error {
	if _, err := s.GetTask(project, agent, t.ID); err != nil {
		return err
	}
	return s.putJSON("tasks", []string{project, agent, t.ID}, t)
}

func (s *DBStore) ListTasks(project, agent string, filter ...entity.TaskStatus) ([]*entity.Task, error) {
	all, err := s.listTasks(project, agent)
	if err != nil {
		return nil, err
	}
	set := map[entity.TaskStatus]bool{}
	for _, f := range filter {
		set[f] = true
	}
	out := make([]*entity.Task, 0, len(all))
	for _, t := range all {
		if t.Status.IsTerminal() {
			continue
		}
		if len(set) > 0 && !set[t.Status] {
			continue
		}
		out = append(out, t)
	}
	sortTasks(out)
	return out, nil
}

func (s *DBStore) ArchiveTask(project, agent string, t *entity.Task) error {
	return s.putJSON("tasks", []string{project, agent, t.ID}, t)
}

func (s *DBStore) ListArchivedTasks(project, agent string) ([]*entity.Task, error) {
	all, err := s.listTasks(project, agent)
	if err != nil {
		return nil, err
	}
	var out []*entity.Task
	for _, t := range all {
		if t.Status.IsTerminal() {
			out = append(out, t)
		}
	}
	sortTasks(out)
	return out, nil
}

func (s *DBStore) OverwriteArchive(project, agent string, tasks []*entity.Task) error {
	archived, err := s.ListArchivedTasks(project, agent)
	if err != nil {
		return err
	}
	for _, t := range archived {
		if err := s.db.DeleteRecord("tasks", s.workspaceID, []string{project, agent, t.ID}); err != nil {
			return err
		}
	}
	for _, t := range tasks {
		if err := s.putJSON("tasks", []string{project, agent, t.ID}, t); err != nil {
			return err
		}
	}
	return nil
}

func (s *DBStore) DeleteTask(project, agent, taskID string) error {
	return s.db.DeleteRecord("tasks", s.workspaceID, []string{project, agent, taskID})
}

func (s *DBStore) ClearTasks(project, agent string) error {
	tasks, err := s.listTasks(project, agent)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if err := s.DeleteTask(project, agent, t.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *DBStore) GetHeartbeat(project, agent string) (*entity.HeartbeatConfig, error) {
	var h entity.HeartbeatConfig
	if ok, err := s.getJSON("heartbeat", []string{project, agent}, &h); err != nil {
		return nil, err
	} else if !ok {
		return &entity.HeartbeatConfig{}, nil
	}
	return &h, nil
}
func (s *DBStore) SaveHeartbeat(project, agent string, h *entity.HeartbeatConfig) error {
	return s.putJSON("heartbeat", []string{project, agent}, h)
}
func (s *DBStore) PauseHeartbeat(project, agent string) error {
	h, err := s.GetHeartbeat(project, agent)
	if err != nil {
		return err
	}
	h.Paused = true
	return s.SaveHeartbeat(project, agent, h)
}
func (s *DBStore) ResumeHeartbeat(project, agent string) error {
	h, err := s.GetHeartbeat(project, agent)
	if err != nil {
		return err
	}
	h.Paused = false
	return s.SaveHeartbeat(project, agent, h)
}

func (s *DBStore) ListCrons(project, agent string) ([]*entity.Cron, error) {
	var crons []*entity.Cron
	if ok, err := s.getJSON("crons", []string{project, agent}, &crons); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	return crons, nil
}
func (s *DBStore) SaveCrons(project, agent string, crons []*entity.Cron) error {
	return s.putJSON("crons", []string{project, agent}, crons)
}
func (s *DBStore) PauseCron(project, agent, cronID string) error {
	return s.setCronEnabled(project, agent, cronID, false)
}
func (s *DBStore) ResumeCron(project, agent, cronID string) error {
	return s.setCronEnabled(project, agent, cronID, true)
}
func (s *DBStore) DeleteCron(project, agent, cronID string) error {
	crons, err := s.ListCrons(project, agent)
	if err != nil {
		return err
	}
	out := crons[:0]
	for _, c := range crons {
		if c.ID != cronID {
			out = append(out, c)
		}
	}
	return s.SaveCrons(project, agent, out)
}

func (s *DBStore) AddToInbox(item *entity.InboxItem) error {
	to := item.Recipient()
	return s.putJSON("inbox", []string{to, item.TaskID}, item)
}
func (s *DBStore) ListInbox() ([]*entity.InboxItem, error) {
	recs, err := s.db.ListRecords("inbox", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*entity.InboxItem, 0, len(recs))
	for _, rec := range recs {
		var item entity.InboxItem
		if json.Unmarshal([]byte(rec.Payload), &item) == nil {
			out = append(out, &item)
		}
	}
	return out, nil
}
func (s *DBStore) RemoveFromInbox(taskID string) error {
	items, err := s.ListInbox()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.TaskID == taskID {
			return s.db.DeleteRecord("inbox", s.workspaceID, []string{item.Recipient(), taskID})
		}
	}
	return nil
}
func (s *DBStore) ClearInbox() error {
	items, err := s.ListInbox()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.db.DeleteRecord("inbox", s.workspaceID, []string{item.Recipient(), item.TaskID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *DBStore) SendMessage(msg *entity.Message) error {
	return s.putJSON("messages", []string{msg.To, msg.ID}, msg)
}
func (s *DBStore) ListMessages(recipient string) ([]*entity.Message, error) {
	all, err := s.ListAllMessages(recipient)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, m := range all {
		if m.ArchivedAt == nil {
			out = append(out, m)
		}
	}
	return out, nil
}
func (s *DBStore) ListUnreadMessages(recipient string) ([]*entity.Message, error) {
	msgs, err := s.ListMessages(recipient)
	if err != nil {
		return nil, err
	}
	out := msgs[:0]
	for _, m := range msgs {
		if m.ReadAt == nil {
			out = append(out, m)
		}
	}
	return out, nil
}
func (s *DBStore) MarkMessagesRead(recipient string) error {
	msgs, err := s.ListAllMessages(recipient)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, m := range msgs {
		if m.ReadAt == nil {
			m.ReadAt = &now
			if err := s.putJSON("messages", []string{recipient, m.ID}, m); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *DBStore) ListAllMessages(recipient string) ([]*entity.Message, error) {
	recs, err := s.db.ListRecords("messages", s.workspaceID, []string{recipient})
	if err != nil {
		return nil, err
	}
	out := make([]*entity.Message, 0, len(recs))
	for _, rec := range recs {
		var msg entity.Message
		if json.Unmarshal([]byte(rec.Payload), &msg) == nil {
			out = append(out, &msg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SentAt.After(out[j].SentAt) })
	return out, nil
}
func (s *DBStore) MarkMessageRead(recipient, msgID string) error {
	return s.updateMessage(recipient, msgID, func(m *entity.Message) { now := time.Now().UTC(); m.ReadAt = &now })
}
func (s *DBStore) ArchiveMessage(recipient, msgID string) error {
	return s.updateMessage(recipient, msgID, func(m *entity.Message) {
		now := time.Now().UTC()
		m.ArchivedAt = &now
		if m.ReadAt == nil {
			m.ReadAt = &now
		}
	})
}
func (s *DBStore) DeleteMessage(recipient, msgID string) error {
	return s.db.DeleteRecord("messages", s.workspaceID, []string{recipient, msgID})
}
func (s *DBStore) ClearMessages(recipient string) error {
	msgs, err := s.ListAllMessages(recipient)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if err := s.DeleteMessage(recipient, m.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *DBStore) ListProjects() ([]string, error) {
	recs, err := s.db.ListRecords("projects", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.Key[0])
	}
	return out, nil
}
func (s *DBStore) ListAgents(project string) ([]string, error) {
	recs, err := s.db.ListRecords("agents", s.workspaceID, []string{project})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(recs))
	for _, rec := range recs {
		out = append(out, rec.Key[1])
	}
	return out, nil
}
func (s *DBStore) FindTaskByID(id string) (string, string, *entity.Task, error) {
	recs, err := s.db.ListRecords("tasks", s.workspaceID, nil)
	if err != nil {
		return "", "", nil, err
	}
	for _, rec := range recs {
		if rec.Key[2] != id {
			continue
		}
		var t entity.Task
		if json.Unmarshal([]byte(rec.Payload), &t) == nil {
			return rec.Key[0], rec.Key[1], &t, nil
		}
	}
	return "", "", nil, errs.NotFound("task", id)
}
func (s *DBStore) ListAllTaskRecords(projectFilter string) ([]TaskRecord, error) {
	prefix := []string{}
	if projectFilter != "" {
		prefix = []string{projectFilter}
	}
	recs, err := s.db.ListRecords("tasks", s.workspaceID, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]TaskRecord, 0, len(recs))
	for _, rec := range recs {
		var t entity.Task
		if json.Unmarshal([]byte(rec.Payload), &t) == nil {
			out = append(out, TaskRecord{Project: rec.Key[0], Agent: rec.Key[1], Task: &t})
		}
	}
	return out, nil
}

func (s *DBStore) AddComment(project, agent string, c *entity.TaskComment) error {
	return s.putJSON("comments", []string{project, agent, c.ID}, c)
}
func (s *DBStore) ListComments(project, agent, taskID string) ([]*entity.TaskComment, error) {
	recs, err := s.db.ListRecords("comments", s.workspaceID, []string{project, agent})
	if err != nil {
		return nil, err
	}
	var out []*entity.TaskComment
	for _, rec := range recs {
		var c entity.TaskComment
		if json.Unmarshal([]byte(rec.Payload), &c) == nil && c.TaskID == taskID {
			out = append(out, &c)
		}
	}
	return out, nil
}
func (s *DBStore) DeleteComment(project, agent, commentID string) error {
	return s.db.DeleteRecord("comments", s.workspaceID, []string{project, agent, commentID})
}
func (s *DBStore) RunLogDir(project, agent string) (string, error) {
	dir := filepath.Join(s.root, "projects", project, "agents", agent, ".multigent", "runs")
	return dir, os.MkdirAll(dir, 0o755)
}

func (s *DBStore) GetProjectConfig(project string) (*entity.ProjectConfig, error) {
	return s.files.GetProjectConfig(project)
}
func (s *DBStore) SaveProjectConfig(project string, cfg *entity.ProjectConfig) error {
	return s.files.SaveProjectConfig(project, cfg)
}
func (s *DBStore) GetProjectBlueprint(name string) (*entity.ProjectConfig, error) {
	return s.files.GetProjectBlueprint(name)
}
func (s *DBStore) ListProjectBlueprints() ([]string, error) {
	return s.files.ListProjectBlueprints()
}

func (s *DBStore) listTasks(project, agent string) ([]*entity.Task, error) {
	recs, err := s.db.ListRecords("tasks", s.workspaceID, []string{project, agent})
	if err != nil {
		return nil, err
	}
	out := make([]*entity.Task, 0, len(recs))
	for _, rec := range recs {
		var t entity.Task
		if json.Unmarshal([]byte(rec.Payload), &t) == nil {
			out = append(out, &t)
		}
	}
	return out, nil
}

func (s *DBStore) setCronEnabled(project, agent, cronID string, enabled bool) error {
	crons, err := s.ListCrons(project, agent)
	if err != nil {
		return err
	}
	for _, c := range crons {
		if c.ID == cronID {
			c.Enabled = enabled
			return s.SaveCrons(project, agent, crons)
		}
	}
	return fmt.Errorf("cron %q not found", cronID)
}

func (s *DBStore) updateMessage(recipient, msgID string, fn func(*entity.Message)) error {
	var msg entity.Message
	if ok, err := s.getJSON("messages", []string{recipient, msgID}, &msg); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("message %q not found", msgID)
	}
	fn(&msg)
	return s.putJSON("messages", []string{recipient, msgID}, &msg)
}

func (s *DBStore) getJSON(table string, key []string, out any) (bool, error) {
	payload, ok, err := s.db.GetRecord(table, s.workspaceID, key)
	if err != nil || !ok {
		return ok, err
	}
	return true, json.Unmarshal([]byte(payload), out)
}
func (s *DBStore) putJSON(table string, key []string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord(table, s.workspaceID, key, string(raw))
}

func sortTasks(tasks []*entity.Task) {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt) })
}

func workspaceID(root string) string {
	absRoot, _ := filepath.Abs(root)
	sum := sha1.Sum([]byte(absRoot))
	return hex.EncodeToString(sum[:])[:12]
}

func ensureWorkspace(root string, db controldb.Store) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	if rows, err := db.ListWorkspaces(); err == nil {
		for _, row := range rows {
			if samePath(row.Root, absRoot) && row.ID != "" {
				return row.ID, nil
			}
		}
	}
	name := filepath.Base(absRoot)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "Multigent Workspace"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := workspaceID(absRoot)
	if base := filepath.Base(absRoot); base != "." && base != string(filepath.Separator) && base != "" {
		id = base
	}
	return id, db.UpsertWorkspace(controldb.Workspace{
		ID:        id,
		Name:      name,
		Slug:      name,
		Root:      absRoot,
		UpdatedAt: now,
	})
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

var _ Store = (*DBStore)(nil)
