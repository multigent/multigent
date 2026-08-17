package contextpack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	ScopeWorkspace = "workspace"
	ScopeProject   = "project"
	ScopeAgent     = "agent"

	CollectorManualUpload = "manual-upload"

	maxCollectedFileBytes = 5 << 20
)

type Source struct {
	ID            string            `yaml:"id" json:"id"`
	CollectorType string            `yaml:"collector_type" json:"collectorType"`
	Name          string            `yaml:"name" json:"name"`
	Config        map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
	CreatedBy     string            `yaml:"created_by,omitempty" json:"createdBy,omitempty"`
	CreatedAt     time.Time         `yaml:"created_at" json:"createdAt"`
	LastScanAt    *time.Time        `yaml:"last_scan_at,omitempty" json:"lastScanAt,omitempty"`
}

type Asset struct {
	ID          string            `yaml:"id" json:"id"`
	SourceID    string            `yaml:"source_id,omitempty" json:"sourceId,omitempty"`
	Title       string            `yaml:"title" json:"title"`
	Kind        string            `yaml:"kind,omitempty" json:"kind,omitempty"`
	MimeType    string            `yaml:"mime_type,omitempty" json:"mimeType,omitempty"`
	StoragePath string            `yaml:"storage_path" json:"storagePath"`
	SHA256      string            `yaml:"sha256" json:"sha256"`
	Size        int64             `yaml:"size" json:"size"`
	CreatedBy   string            `yaml:"created_by,omitempty" json:"createdBy,omitempty"`
	CreatedAt   time.Time         `yaml:"created_at" json:"createdAt"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type Artifact struct {
	ID               string    `yaml:"id" json:"id"`
	AssetID          string    `yaml:"asset_id,omitempty" json:"assetId,omitempty"`
	DocID            string    `yaml:"doc_id" json:"docId"`
	Title            string    `yaml:"title" json:"title"`
	Kind             string    `yaml:"kind,omitempty" json:"kind,omitempty"`
	Summary          string    `yaml:"summary,omitempty" json:"summary,omitempty"`
	Processor        string    `yaml:"processor,omitempty" json:"processor,omitempty"`
	ProcessorVersion string    `yaml:"processor_version,omitempty" json:"processorVersion,omitempty"`
	CreatedAt        time.Time `yaml:"created_at" json:"createdAt"`
}

type Binding struct {
	ID         string    `yaml:"id" json:"id"`
	ArtifactID string    `yaml:"artifact_id" json:"artifactId"`
	DocID      string    `yaml:"doc_id,omitempty" json:"docId,omitempty"`
	ScopeType  string    `yaml:"scope_type" json:"scopeType"`
	ScopeID    string    `yaml:"scope_id,omitempty" json:"scopeId,omitempty"`
	Mode       string    `yaml:"mode,omitempty" json:"mode,omitempty"`
	Required   bool      `yaml:"required,omitempty" json:"required,omitempty"`
	Priority   int       `yaml:"priority,omitempty" json:"priority,omitempty"`
	CreatedBy  string    `yaml:"created_by,omitempty" json:"createdBy,omitempty"`
	CreatedAt  time.Time `yaml:"created_at" json:"createdAt"`
}

type BindingView struct {
	Binding  Binding         `json:"binding"`
	Artifact Artifact        `json:"artifact"`
	Doc      *store.DocEntry `json:"doc,omitempty"`
}

type Index struct {
	Sources   []Source   `yaml:"sources,omitempty" json:"sources,omitempty"`
	Assets    []Asset    `yaml:"assets,omitempty" json:"assets,omitempty"`
	Artifacts []Artifact `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
	Bindings  []Binding  `yaml:"bindings,omitempty" json:"bindings,omitempty"`
}

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) indexPath() string {
	return filepath.Join(s.root, ".multigent", "context-index.yaml")
}

func (s *Store) assetDir() string {
	return filepath.Join(s.root, ".multigent", "context-assets")
}

func (s *Store) Load() (*Index, error) {
	raw, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{}, nil
		}
		return nil, err
	}
	var idx Index
	if err := yaml.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func (s *Store) Save(idx *Index) error {
	if idx == nil {
		idx = &Index{}
	}
	if err := os.MkdirAll(filepath.Dir(s.indexPath()), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), raw, 0o644)
}

type ImportManualInput struct {
	Title       string
	Content     string
	SourceName  string
	CreatedBy   string
	Project     string
	Tags        []string
	Description string
	BindScope   string
	BindScopeID string
	Required    bool
}

type ImportFileInput struct {
	FilePath    string
	Title       string
	CreatedBy   string
	Project     string
	Tags        []string
	Description string
	BindScope   string
	BindScopeID string
	Required    bool
}

type ImportLocalPathInput struct {
	Path          string
	CollectorType string
	Title         string
	CreatedBy     string
	Project       string
	Tags          []string
	Description   string
	BindScope     string
	BindScopeID   string
	Required      bool
	Metadata      map[string]string
}

type ImportContentInput struct {
	Path          string
	CollectorType string
	Title         string
	Content       string
	SourceName    string
	FilePath      string
	CreatedBy     string
	Project       string
	Tags          []string
	Description   string
	BindScope     string
	BindScopeID   string
	Required      bool
	Metadata      map[string]string
}

type ImportManualResult struct {
	Source   Source          `json:"source"`
	Asset    Asset           `json:"asset"`
	Artifact Artifact        `json:"artifact"`
	Binding  *Binding        `json:"binding,omitempty"`
	Doc      *store.DocEntry `json:"doc"`
}

func (s *Store) ImportManual(input ImportManualInput) (*ImportManualResult, error) {
	return s.ImportContent(ImportContentInput{
		CollectorType: CollectorManualUpload,
		Title:         input.Title,
		Content:       input.Content,
		SourceName:    input.SourceName,
		Project:       input.Project,
		Tags:          input.Tags,
		Description:   input.Description,
		CreatedBy:     input.CreatedBy,
		BindScope:     input.BindScope,
		BindScopeID:   input.BindScopeID,
		Required:      input.Required,
	})
}

func (s *Store) ImportFile(input ImportFileInput) (*ImportManualResult, error) {
	rel := strings.TrimSpace(input.FilePath)
	if rel == "" {
		return nil, fmt.Errorf("filePath is required")
	}
	abs, ok := s.resolveManagedFilePath(rel)
	if !ok {
		return nil, fmt.Errorf("invalid filePath")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("filePath must be a file")
	}
	if info.Size() > maxCollectedFileBytes {
		return nil, fmt.Errorf("file is too large for knowledge import: %d bytes", info.Size())
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if !looksText(raw) {
		return nil, fmt.Errorf("only text files can be imported as agent reference material")
	}
	items, err := NewRegistry().Collect(context.Background(), CollectorLocalFile, CollectInput{
		Title:       input.Title,
		Content:     string(raw),
		SourceName:  filepath.Base(rel),
		FilePath:    rel,
		Project:     input.Project,
		Tags:        input.Tags,
		Description: input.Description,
		CreatedBy:   input.CreatedBy,
		Metadata:    map[string]string{"filePath": rel},
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("collector returned no items")
	}
	return s.importCollectedItem(CollectorLocalFile, input.CreatedBy, input.Project, items[0], Binding{
		ScopeType: strings.TrimSpace(input.BindScope),
		ScopeID:   strings.TrimSpace(input.BindScopeID),
		Required:  input.Required,
	})
}

func (s *Store) ImportLocalPath(input ImportLocalPathInput) (*ImportManualResult, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path must be a file")
	}
	limit := maxCollectedFileBytes
	if strings.TrimSpace(input.CollectorType) == CollectorLocalAgentSession {
		limit = maxSessionImportBytes
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("file is too large for import: %d bytes", info.Size())
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if !looksText(raw) {
		return nil, fmt.Errorf("only text files can be imported as agent reference material")
	}
	collectorType := strings.TrimSpace(input.CollectorType)
	if collectorType == "" {
		collectorType = CollectorLocalFile
	}
	meta := cloneMap(input.Metadata)
	meta["originalPath"] = abs
	return s.ImportContent(ImportContentInput{
		CollectorType: collectorType,
		Title:         input.Title,
		Content:       string(raw),
		SourceName:    filepath.Base(abs),
		FilePath:      abs,
		Project:       input.Project,
		Tags:          input.Tags,
		Description:   input.Description,
		CreatedBy:     input.CreatedBy,
		Metadata:      meta,
		BindScope:     input.BindScope,
		BindScopeID:   input.BindScopeID,
		Required:      input.Required,
	})
}

func (s *Store) ImportContent(input ImportContentInput) (*ImportManualResult, error) {
	collectorType := strings.TrimSpace(input.CollectorType)
	if collectorType == "" {
		collectorType = CollectorManualUpload
	}
	limit := maxCollectedFileBytes
	if collectorType == CollectorLocalAgentSession {
		limit = maxSessionImportBytes
	}
	raw := []byte(input.Content)
	if len(raw) > limit {
		return nil, fmt.Errorf("content is too large for import: %d bytes", len(raw))
	}
	if !looksText(raw) {
		return nil, fmt.Errorf("only text content can be imported as agent reference material")
	}
	items, err := NewRegistry().Collect(context.Background(), collectorType, CollectInput{
		Title:       input.Title,
		Content:     input.Content,
		SourceName:  input.SourceName,
		FilePath:    input.FilePath,
		Project:     input.Project,
		Tags:        input.Tags,
		Description: input.Description,
		CreatedBy:   input.CreatedBy,
		Metadata:    cloneMap(input.Metadata),
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("collector returned no items")
	}
	return s.importCollectedItem(collectorType, input.CreatedBy, input.Project, items[0], Binding{
		ScopeType: strings.TrimSpace(input.BindScope),
		ScopeID:   strings.TrimSpace(input.BindScopeID),
		Required:  input.Required,
	})
}

func (s *Store) importCollectedItem(collectorType, createdBy, project string, item CollectedItem, bindingTemplate Binding) (*ImportManualResult, error) {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	title := normalizeTitle(item.Title, item.SourceName)
	now := time.Now().UTC()
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	source := s.ensureSource(idx, collectorType, createdBy, now)
	sum := sha256.Sum256([]byte(content))
	assetID := newID("ctx-asset")
	ext := safeAssetExt(filepath.Ext(item.SourceName))
	if ext == "" {
		ext = ".md"
	}
	if err := os.MkdirAll(s.assetDir(), 0o755); err != nil {
		return nil, err
	}
	fileName := assetID + "-" + slug(title) + ext
	relPath := filepath.Join(".multigent", "context-assets", fileName)
	if err := os.WriteFile(filepath.Join(s.root, relPath), []byte(content), 0o644); err != nil {
		return nil, err
	}
	asset := Asset{
		ID:          assetID,
		SourceID:    source.ID,
		Title:       title,
		Kind:        firstNonEmpty(item.Kind, collectorType),
		MimeType:    firstNonEmpty(item.MimeType, mimeFromExt(ext)),
		StoragePath: relPath,
		SHA256:      hex.EncodeToString(sum[:]),
		Size:        int64(len([]byte(content))),
		CreatedBy:   createdBy,
		CreatedAt:   now,
		Metadata:    mergeStringMaps(map[string]string{"sourceName": item.SourceName}, item.Metadata),
	}
	idx.Assets = append(idx.Assets, asset)

	doc := &store.DocEntry{
		Title:       title,
		Index:       contextDocIndex(project),
		CreatedBy:   createdBy,
		Tags:        normalizeTags(append([]string{"agent-reference"}, item.Tags...)),
		Description: firstNonEmpty(item.Description, "Imported reference material."),
	}
	ds := store.NewDocsStore(s.root)
	if err := ds.AddManagedContent(doc, content, item.SourceName); err != nil {
		return nil, err
	}
	artifact := Artifact{
		ID:               newID("ctx-artifact"),
		AssetID:          asset.ID,
		DocID:            doc.ID,
		Title:            title,
		Kind:             "knowledge-doc",
		Summary:          item.Description,
		Processor:        "text-to-knowledge-doc",
		ProcessorVersion: "v1",
		CreatedAt:        now,
	}
	idx.Artifacts = append(idx.Artifacts, artifact)

	var binding *Binding
	if strings.TrimSpace(bindingTemplate.ScopeType) != "" {
		b := Binding{
			ID:         newID("ctx-binding"),
			ArtifactID: artifact.ID,
			DocID:      doc.ID,
			ScopeType:  normalizeScope(bindingTemplate.ScopeType),
			ScopeID:    strings.TrimSpace(bindingTemplate.ScopeID),
			Mode:       firstNonEmpty(bindingTemplate.Mode, "reference"),
			Required:   bindingTemplate.Required,
			Priority:   bindingTemplate.Priority,
			CreatedBy:  createdBy,
			CreatedAt:  now,
		}
		if err := validateBinding(b); err != nil {
			return nil, err
		}
		idx.Bindings = append(idx.Bindings, b)
		binding = &b
	}
	if err := s.Save(idx); err != nil {
		return nil, err
	}
	return &ImportManualResult{Source: source, Asset: asset, Artifact: artifact, Binding: binding, Doc: doc}, nil
}

func (s *Store) ensureManualSource(idx *Index, createdBy string, now time.Time) Source {
	return s.ensureSource(idx, CollectorManualUpload, createdBy, now)
}

func (s *Store) ensureSource(idx *Index, collectorType, createdBy string, now time.Time) Source {
	collectorType = strings.TrimSpace(collectorType)
	if collectorType == "" {
		collectorType = CollectorManualUpload
	}
	for _, source := range idx.Sources {
		if source.CollectorType == collectorType {
			return source
		}
	}
	name := collectorType
	for _, spec := range NewRegistry().Specs() {
		if spec.Type == collectorType {
			name = spec.Name
			break
		}
	}
	source := Source{
		ID:            newID("ctx-source"),
		CollectorType: collectorType,
		Name:          name,
		CreatedBy:     createdBy,
		CreatedAt:     now,
	}
	idx.Sources = append(idx.Sources, source)
	return source
}

func (s *Store) AddBinding(binding Binding) (*Binding, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	if binding.ID == "" {
		binding.ID = newID("ctx-binding")
	}
	binding.ScopeType = normalizeScope(binding.ScopeType)
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = time.Now().UTC()
	}
	if binding.Mode == "" {
		binding.Mode = "reference"
	}
	if binding.DocID == "" {
		for _, artifact := range idx.Artifacts {
			if artifact.ID == binding.ArtifactID {
				binding.DocID = artifact.DocID
				break
			}
		}
	}
	if err := validateBinding(binding); err != nil {
		return nil, err
	}
	if binding.ArtifactID == "" && binding.DocID != "" {
		artifact, err := s.ensureDocArtifact(idx, binding.DocID)
		if err != nil {
			return nil, err
		}
		binding.ArtifactID = artifact.ID
	}
	for _, existing := range idx.Bindings {
		sameArtifact := binding.ArtifactID != "" && existing.ArtifactID == binding.ArtifactID
		sameDoc := binding.DocID != "" && existing.DocID == binding.DocID
		if (sameArtifact || sameDoc) && existing.ScopeType == binding.ScopeType && existing.ScopeID == binding.ScopeID {
			return nil, fmt.Errorf("context binding already exists")
		}
	}
	idx.Bindings = append(idx.Bindings, binding)
	if err := s.Save(idx); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *Store) ensureDocArtifact(idx *Index, docID string) (*Artifact, error) {
	docID = strings.TrimSpace(docID)
	for i := range idx.Artifacts {
		if idx.Artifacts[i].DocID == docID {
			return &idx.Artifacts[i], nil
		}
	}
	doc, err := store.NewDocsStore(s.root).Get(docID)
	if err != nil {
		return nil, err
	}
	artifact := Artifact{
		ID:               newID("ctx-artifact"),
		DocID:            doc.ID,
		Title:            doc.Title,
		Kind:             "knowledge-doc",
		Summary:          doc.Description,
		Processor:        "existing-knowledge-doc",
		ProcessorVersion: "v1",
		CreatedAt:        time.Now().UTC(),
	}
	idx.Artifacts = append(idx.Artifacts, artifact)
	return &idx.Artifacts[len(idx.Artifacts)-1], nil
}

func (s *Store) RemoveBinding(id string) error {
	idx, err := s.Load()
	if err != nil {
		return err
	}
	out := idx.Bindings[:0]
	found := false
	for _, binding := range idx.Bindings {
		if binding.ID == id {
			found = true
			continue
		}
		out = append(out, binding)
	}
	if !found {
		return fmt.Errorf("context binding %q not found", id)
	}
	idx.Bindings = out
	return s.Save(idx)
}

func (s *Store) ListBindingViews(scopes []ScopeRef) ([]BindingView, error) {
	idx, err := s.Load()
	if err != nil {
		return nil, err
	}
	artifactByID := map[string]Artifact{}
	for _, artifact := range idx.Artifacts {
		artifactByID[artifact.ID] = artifact
	}
	ds := store.NewDocsStore(s.root)
	var views []BindingView
	for _, binding := range idx.Bindings {
		if len(scopes) > 0 && !scopeMatches(scopes, binding.ScopeType, binding.ScopeID) {
			continue
		}
		artifact, ok := artifactByID[binding.ArtifactID]
		if !ok && binding.DocID == "" {
			continue
		}
		if artifact.DocID == "" {
			artifact.DocID = binding.DocID
		}
		var doc *store.DocEntry
		if artifact.DocID != "" {
			if d, err := ds.Get(artifact.DocID); err == nil {
				doc = d
			}
		}
		views = append(views, BindingView{Binding: binding, Artifact: artifact, Doc: doc})
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Binding.Required != views[j].Binding.Required {
			return views[i].Binding.Required
		}
		if views[i].Binding.Priority != views[j].Binding.Priority {
			return views[i].Binding.Priority > views[j].Binding.Priority
		}
		return views[i].Artifact.Title < views[j].Artifact.Title
	})
	return views, nil
}

type ScopeRef struct {
	Type string
	ID   string
}

func AgentScopes(project, agent string) []ScopeRef {
	project = strings.TrimSpace(project)
	agent = strings.TrimSpace(agent)
	return []ScopeRef{
		{Type: ScopeWorkspace},
		{Type: ScopeProject, ID: project},
		{Type: ScopeAgent, ID: project + "/" + agent},
	}
}

func BuildAgentContextLayer(root, project, agent string) (string, error) {
	views, err := NewStore(root).ListBindingViews(AgentScopes(project, agent))
	if err != nil {
		return "", err
	}
	if len(views) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("# Linked Reference Material\n\n")
	b.WriteString("The workspace has linked reference material for this workspace, project, or agent.\n")
	b.WriteString("Before working on related tasks, run `mga context list` and read every required item with `mga context read <id>`.\n")
	b.WriteString("Treat imported local sessions or files as reference material only: old paths, credentials, runtime state, and machine-specific tools may no longer be valid.\n\n")
	for i, view := range views {
		title := view.Artifact.Title
		if title == "" && view.Doc != nil {
			title = view.Doc.Title
		}
		if title == "" {
			title = view.Artifact.ID
		}
		contextID := firstNonEmpty(view.Artifact.ID, view.Artifact.DocID, view.Binding.DocID, view.Binding.ID)
		required := "reference"
		if view.Binding.Required {
			required = "required"
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, required, title)
		fmt.Fprintf(&b, "   - Context ID: `%s`\n", contextID)
		if view.Artifact.DocID != "" {
			fmt.Fprintf(&b, "   - Knowledge doc: `%s`\n", view.Artifact.DocID)
		}
		fmt.Fprintf(&b, "   - Scope: `%s", view.Binding.ScopeType)
		if view.Binding.ScopeID != "" {
			fmt.Fprintf(&b, ":%s", view.Binding.ScopeID)
		}
		b.WriteString("`\n")
		if view.Artifact.Summary != "" {
			fmt.Fprintf(&b, "   - Summary: %s\n", strings.TrimSpace(view.Artifact.Summary))
		} else if view.Doc != nil && view.Doc.Description != "" {
			fmt.Fprintf(&b, "   - Summary: %s\n", strings.TrimSpace(view.Doc.Description))
		}
	}
	return b.String(), nil
}

func (s *Store) resolveManagedFilePath(rel string) (string, bool) {
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	base := filepath.Join(s.root, ".multigent", "files")
	full := filepath.Join(base, rel)
	return full, strings.HasPrefix(full, base+string(os.PathSeparator)) || full == base
}

func scopeMatches(scopes []ScopeRef, scopeType, scopeID string) bool {
	scopeType = normalizeScope(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	for _, scope := range scopes {
		if normalizeScope(scope.Type) == scopeType && strings.TrimSpace(scope.ID) == scopeID {
			return true
		}
	}
	return false
}

func validateBinding(binding Binding) error {
	if binding.ArtifactID == "" && binding.DocID == "" {
		return fmt.Errorf("artifactId or docId is required")
	}
	switch normalizeScope(binding.ScopeType) {
	case ScopeWorkspace:
		return nil
	case ScopeProject, ScopeAgent:
		if strings.TrimSpace(binding.ScopeID) == "" {
			return fmt.Errorf("scopeId is required for %s binding", binding.ScopeType)
		}
		return nil
	default:
		return fmt.Errorf("unsupported scopeType %q", binding.ScopeType)
	}
}

func normalizeScope(scope string) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "", ScopeWorkspace:
		return ScopeWorkspace
	case ScopeProject:
		return ScopeProject
	case ScopeAgent:
		return ScopeAgent
	default:
		return strings.TrimSpace(strings.ToLower(scope))
	}
}

func contextDocIndex(project string) string {
	project = strings.Trim(strings.TrimSpace(project), "/")
	if project == "" {
		return "context"
	}
	return "projects/" + project + "/context"
}

func normalizeTitle(title, sourceName string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(sourceName), filepath.Ext(sourceName))
	}
	if title == "" || title == "." {
		title = "Imported Reference"
	}
	return title
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func mergeStringMaps(base, extra map[string]string) map[string]string {
	out := cloneMap(base)
	for k, v := range extra {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func looksText(raw []byte) bool {
	for _, b := range raw {
		if b == 0 {
			return false
		}
	}
	return true
}

func newID(prefix string) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102"), string(b))
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	var out []rune
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			out = append(out, r)
			lastDash = false
			continue
		}
		if !lastDash {
			out = append(out, '-')
			lastDash = true
		}
	}
	res := strings.Trim(string(out), "-_")
	if res == "" {
		return "context"
	}
	if len(res) > 80 {
		res = res[:80]
	}
	return res
}

func safeAssetExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".md", ".markdown", ".txt", ".json", ".jsonl", ".log", ".yaml", ".yml", ".csv":
		return ext
	default:
		return ""
	}
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".json", ".jsonl":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".csv":
		return "text/csv"
	default:
		return "text/plain"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
