package contextpack

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	CollectorLocalFile = "local-file"
)

type CollectorSpec struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type CollectInput struct {
	Title       string
	Content     string
	SourceName  string
	FilePath    string
	Project     string
	Tags        []string
	Description string
	CreatedBy   string
	Metadata    map[string]string
}

type CollectedItem struct {
	Title       string
	Content     string
	SourceName  string
	Kind        string
	MimeType    string
	Tags        []string
	Description string
	Metadata    map[string]string
}

type Collector interface {
	Spec() CollectorSpec
	Collect(ctx context.Context, input CollectInput) ([]CollectedItem, error)
}

type Registry struct {
	collectors map[string]Collector
}

func NewRegistry() *Registry {
	r := &Registry{collectors: map[string]Collector{}}
	r.Register(ManualUploadCollector{})
	r.Register(LocalFileCollector{})
	r.Register(LocalAgentSessionCollector{})
	return r
}

func (r *Registry) Register(collector Collector) {
	if collector == nil {
		return
	}
	spec := collector.Spec()
	key := strings.TrimSpace(spec.Type)
	if key == "" {
		return
	}
	r.collectors[key] = collector
}

func (r *Registry) Specs() []CollectorSpec {
	specs := make([]CollectorSpec, 0, len(r.collectors))
	for _, collector := range r.collectors {
		specs = append(specs, collector.Spec())
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Type < specs[j].Type })
	return specs
}

func (r *Registry) Collect(ctx context.Context, collectorType string, input CollectInput) ([]CollectedItem, error) {
	collector, ok := r.collectors[strings.TrimSpace(collectorType)]
	if !ok {
		return nil, fmt.Errorf("unsupported collector %q", collectorType)
	}
	return collector.Collect(ctx, input)
}

type ManualUploadCollector struct{}

func (ManualUploadCollector) Spec() CollectorSpec {
	return CollectorSpec{
		Type:        CollectorManualUpload,
		Name:        "Manual upload",
		Description: "Import pasted text or uploaded text content into the knowledge base.",
	}
}

func (ManualUploadCollector) Collect(ctx context.Context, input CollectInput) ([]CollectedItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.SourceName)
	}
	return []CollectedItem{{
		Title:       title,
		Content:     content,
		SourceName:  input.SourceName,
		Kind:        "manual",
		Tags:        input.Tags,
		Description: input.Description,
		Metadata:    cloneMap(input.Metadata),
	}}, nil
}

type LocalAgentSessionCollector struct{}

func (LocalAgentSessionCollector) Spec() CollectorSpec {
	return CollectorSpec{
		Type:        CollectorLocalAgentSession,
		Name:        "Local agent session",
		Description: "Import a local Claude Code, Codex, Cursor, or compatible agent session file as knowledge-base reference material.",
	}
}

func (LocalAgentSessionCollector) Collect(ctx context.Context, input CollectInput) ([]CollectedItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	meta := cloneMap(input.Metadata)
	if strings.TrimSpace(input.FilePath) != "" {
		meta["filePath"] = strings.TrimSpace(input.FilePath)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.SourceName)
	}
	return []CollectedItem{{
		Title:       title,
		Content:     content,
		SourceName:  input.SourceName,
		Kind:        "agent-session",
		Tags:        append([]string{"agent-session"}, input.Tags...),
		Description: input.Description,
		Metadata:    meta,
	}}, nil
}

type LocalFileCollector struct{}

func (LocalFileCollector) Spec() CollectorSpec {
	return CollectorSpec{
		Type:        CollectorLocalFile,
		Name:        "Workspace file",
		Description: "Import a text file already stored in the workspace file manager.",
	}
}

func (LocalFileCollector) Collect(ctx context.Context, input CollectInput) ([]CollectedItem, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	meta := cloneMap(input.Metadata)
	if strings.TrimSpace(input.FilePath) != "" {
		meta["filePath"] = strings.TrimSpace(input.FilePath)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.SourceName)
	}
	return []CollectedItem{{
		Title:       title,
		Content:     content,
		SourceName:  input.SourceName,
		Kind:        "file",
		Tags:        input.Tags,
		Description: input.Description,
		Metadata:    meta,
	}}, nil
}
