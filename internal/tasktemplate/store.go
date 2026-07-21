package tasktemplate

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/entity"
)

type Store struct {
	db          controldb.Store
	workspaceID string
}

func NewStore(db controldb.Store, workspaceID string) *Store {
	return &Store{db: db, workspaceID: workspaceID}
}

func (s *Store) Save(template *entity.TaskTemplate) error {
	if template.ID == "" {
		template.ID = entity.NewTaskTemplateID()
	}
	now := time.Now().UTC()
	if template.CreatedAt.IsZero() {
		template.CreatedAt = now
	}
	template.UpdatedAt = now
	if template.Type == "" {
		template.Type = string(entity.TaskTypeChore)
	}
	if template.Priority < 0 || template.Priority > 3 {
		template.Priority = 2
	}
	raw, err := json.Marshal(template)
	if err != nil {
		return err
	}
	return s.db.UpsertRecord("task_templates", s.workspaceID, []string{template.ID}, string(raw))
}

func (s *Store) List() ([]entity.TaskTemplate, error) {
	recs, err := s.db.ListRecords("task_templates", s.workspaceID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]entity.TaskTemplate, 0, len(recs))
	for _, rec := range recs {
		var template entity.TaskTemplate
		if json.Unmarshal([]byte(rec.Payload), &template) == nil {
			out = append(out, template)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) Get(id string) (entity.TaskTemplate, bool, error) {
	raw, ok, err := s.db.GetRecord("task_templates", s.workspaceID, []string{id})
	if err != nil || !ok {
		return entity.TaskTemplate{}, ok, err
	}
	var template entity.TaskTemplate
	if err := json.Unmarshal([]byte(raw), &template); err != nil {
		return entity.TaskTemplate{}, false, err
	}
	return template, true, nil
}

func (s *Store) Delete(id string) error {
	return s.db.DeleteRecord("task_templates", s.workspaceID, []string{id})
}
