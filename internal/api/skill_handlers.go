package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/errs"
	"gopkg.in/yaml.v3"
)

type skillRow struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description,omitempty"`
	Provenance  *entity.PlaybookObjectProvenance `json:"provenance,omitempty"`
	Source      string                           `json:"source,omitempty"`
	SourceType  string                           `json:"sourceType,omitempty"`
	SourceRef   string                           `json:"sourceRef,omitempty"`
	Version     string                           `json:"version,omitempty"`
	Managed     bool                             `json:"managed,omitempty"`
	Dirty       bool                             `json:"dirty,omitempty"`
	InstalledAt string                           `json:"installedAt,omitempty"`
	UpdatedAt   string                           `json:"updatedAt,omitempty"`
}

type createSkillBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type skillFileEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode,omitempty"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

type skillInstallBody struct {
	Source      string `json:"source"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Managed     *bool  `json:"managed,omitempty"`
}

type skillPackageRow struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
	SourceType  string `json:"sourceType,omitempty"`
	SourceRef   string `json:"sourceRef,omitempty"`
	Version     string `json:"version"`
	Managed     bool   `json:"managed,omitempty"`
	Dirty       bool   `json:"dirty,omitempty"`
	InstalledAt string `json:"installedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Path        string `json:"path,omitempty"`
	Installed   bool   `json:"installed,omitempty"`
}

type skillPublishBody struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Source      string           `json:"source,omitempty"`
	SourceType  string           `json:"sourceType,omitempty"`
	SourceRef   string           `json:"sourceRef,omitempty"`
	Managed     *bool            `json:"managed,omitempty"`
	Files       []skillFileEntry `json:"files"`
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	skills, err := s.st.ListSkills()
	if err != nil {
		s.serverError(w, err)
		return
	}
	workspaceID, _ := s.currentWorkspaceID()
	provenance, _ := s.playbookProvenanceMap(workspaceID, "skill")
	out := make([]skillRow, 0, len(skills))
	for _, sk := range skills {
		if sk == nil {
			continue
		}
		var prov *entity.PlaybookObjectProvenance
		if p, ok := provenance[playbookProvenanceKey("", sk.Name)]; ok {
			cp := p
			prov = &cp
		}
		meta := s.skillRegistryMeta(sk.Name)
		out = append(out, skillRow{
			Name:        sk.Name,
			Description: sk.Description,
			Provenance:  prov,
			Source:      firstNonEmpty(meta.Source, sk.Source),
			SourceType:  firstNonEmpty(meta.SourceType, sk.SourceType),
			SourceRef:   firstNonEmpty(meta.SourceRef, sk.SourceRef),
			Version:     firstNonEmpty(meta.Version, sk.Version),
			Managed:     meta.Managed || sk.Managed,
			Dirty:       meta.Dirty || sk.Dirty,
			InstalledAt: firstNonEmpty(meta.InstalledAt, sk.InstalledAt),
			UpdatedAt:   firstNonEmpty(meta.UpdatedAt, sk.UpdatedAt),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleGetSkillDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := s.st.Skill(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		s.serverError(w, err)
		return
	}
	prompt, err := s.st.SkillPrompt(name)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":        sk.Name,
		"description": sk.Description,
		"prompt":      prompt,
		"dir":         s.st.SkillDir(name),
		"provenance":  s.playbookObjectProvenanceForRequest(r, "skill", "", name),
		"registry":    s.skillRegistryMeta(name),
		"packageDir":  s.skillPackageDirForMeta(name, s.skillRegistryMeta(name)),
	})
}

func (s *Server) handleGetSkillFiles(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.st.Skill(name); err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		s.serverError(w, err)
		return
	}
	files, err := skillFileTree(s.st.SkillDir(name), true)
	if err != nil {
		s.serverError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":  name,
		"files": files,
	})
}

func (s *Server) handleListSkillRegistry(w http.ResponseWriter, r *http.Request) {
	packages, err := s.listSkillRegistryPackages()
	if err != nil {
		s.serverError(w, err)
		return
	}
	installed := map[string]bool{}
	if skills, err := s.st.ListSkills(); err == nil {
		for _, sk := range skills {
			if sk == nil {
				continue
			}
			meta := s.skillRegistryMeta(sk.Name)
			version := firstNonEmpty(meta.Version, sk.Version)
			if version != "" {
				installed[sk.Name+"@"+version] = true
			}
		}
	}
	for i := range packages {
		packages[i].Installed = installed[packages[i].Name+"@"+packages[i].Version]
	}
	_ = json.NewEncoder(w).Encode(packages)
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body createSkillBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if _, err := s.st.Skill(name); err == nil {
		s.jsonErrorCode(w, http.StatusConflict, ErrCodeConflict, fmt.Sprintf("skill %q already exists", name))
		return
	} else if !isNotFoundErr(err) {
		s.serverError(w, err)
		return
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	description := strings.TrimSpace(body.Description)
	if description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", description))
	}
	sb.WriteString("---\n\n")
	content := normalizeUploadedSkillContent(body.Content)
	if content == "" {
		sb.WriteString(fmt.Sprintf("# Skill: %s\n\n", name))
		sb.WriteString("Describe when to use this skill, the workflow to follow, and any constraints.\n")
	} else {
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
	}

	skillDir := s.st.SkillDir(name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		s.serverError(w, err)
		return
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(sb.String()), 0o644); err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	version := skillVersionFromRef(now)
	meta := entity.Skill{
		Name:        name,
		Description: description,
		SourceType:  "manual",
		Version:     version,
		Managed:     false,
		Dirty:       false,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	_ = writeSkillRegistryMeta(skillDir, meta)
	_ = s.snapshotSkillPackage(skillDir, meta)
	s.auditLog(auditLogInput{
		Action:       "skill.create",
		ResourceType: "skill",
		ResourceID:   name,
		Summary:      "Skill created",
		After: map[string]any{
			"name":        name,
			"description": description,
		},
		Request: r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"skill": name,
	})
}

func (s *Server) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	if !s.checkCurrentWorkspaceAdmin(w, r) {
		return
	}
	var body skillInstallBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "source is required")
		return
	}
	managed := true
	if body.Managed != nil {
		managed = *body.Managed
	}

	if registryName, registryVersion, ok := parseRegistrySkillSource(source); ok {
		if err := s.installSkillPackageReference(registryName, registryVersion, strings.TrimSpace(body.Name)); err != nil {
			if isNotFoundErr(err) {
				s.jsonErrorCode(w, http.StatusNotFound, ErrCodeNotFound, err.Error())
				return
			}
			s.serverError(w, err)
			return
		}
		s.auditLog(auditLogInput{
			Action:       "skill.install",
			ResourceType: "skill",
			ResourceID:   firstNonEmpty(strings.TrimSpace(body.Name), registryName),
			Summary:      "Skill installed from registry",
			After:        map[string]any{"name": firstNonEmpty(strings.TrimSpace(body.Name), registryName), "source": source, "version": registryVersion},
			Request:      r,
		})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skill": firstNonEmpty(strings.TrimSpace(body.Name), registryName), "version": registryVersion})
		return
	}

	tmp := ""
	srcDir := source
	sourceType := "local"
	sourceRef := ""
	if looksLikeGitSource(source) {
		var err error
		tmp, err = os.MkdirTemp("", "multigent-skill-*")
		if err != nil {
			s.serverError(w, err)
			return
		}
		defer os.RemoveAll(tmp)
		if err := cloneSkillSource(source, tmp); err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
			return
		}
		srcDir = tmp
		sourceType = "git"
		sourceRef = gitHead(srcDir)
	}

	if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "source must be a skill directory or git URL")
		return
	}
	name, description, installDir, err := resolveSkillInstallSource(srcDir, body.Name, body.Description)
	if err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	dst := s.st.SkillDir(name)
	if err := os.RemoveAll(dst); err != nil {
		s.serverError(w, err)
		return
	}
	if err := copyDir(installDir, dst); err != nil {
		s.serverError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	version := firstNonEmpty(skillVersionFromRef(sourceRef), skillVersionFromRef(now))
	meta := entity.Skill{
		Name:        name,
		Description: description,
		Source:      source,
		SourceType:  sourceType,
		SourceRef:   sourceRef,
		Version:     version,
		Managed:     managed,
		Dirty:       false,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	if err := writeSkillRegistryMeta(dst, meta); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.snapshotSkillPackage(dst, meta); err != nil {
		s.serverError(w, err)
		return
	}
	if managed {
		if err := s.installSkillPackageReference(name, version, name); err != nil {
			s.serverError(w, err)
			return
		}
	}
	s.auditLog(auditLogInput{
		Action:       "skill.install",
		ResourceType: "skill",
		ResourceID:   name,
		Summary:      "Skill installed",
		After:        map[string]any{"name": name, "source": source, "sourceType": sourceType, "sourceRef": sourceRef},
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skill": name, "sourceRef": sourceRef})
}

func (s *Server) handleRuntimeSkillPublish(w http.ResponseWriter, r *http.Request) {
	principal, ok := runtimeAgentFromRequest(r)
	if !ok {
		s.jsonErrorCode(w, http.StatusUnauthorized, ErrCodeRuntimeAgentTokenRequired, "runtime agent token required")
		return
	}
	if !runtimeHasCapability(principal, "skill.publish") && !runtimeHasCapability(principal, "task.write") {
		s.jsonErrorCode(w, http.StatusForbidden, ErrCodeRuntimeCapabilityRequired, "runtime token lacks skill.publish capability")
		return
	}
	var body skillPublishBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeInvalidJSON, "invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
		return
	}
	if len(body.Files) == 0 {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "files are required")
		return
	}
	hasSkillMD := false
	dst := s.st.SkillDir(name)
	if err := os.RemoveAll(dst); err != nil {
		s.serverError(w, err)
		return
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		s.serverError(w, err)
		return
	}
	for _, f := range body.Files {
		rel, err := cleanSkillRelativePath(f.Path)
		if err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
			return
		}
		if rel == "SKILL.md" {
			hasSkillMD = true
		}
		raw, err := decodeSkillFileContent(f)
		if err != nil {
			s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, err.Error())
			return
		}
		path := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			s.serverError(w, err)
			return
		}
		mode := os.FileMode(0o644)
		if strings.Contains(f.Mode, "x") || strings.HasSuffix(rel, ".sh") || strings.HasSuffix(rel, ".bash") {
			mode = 0o755
		}
		if err := os.WriteFile(path, raw, mode); err != nil {
			s.serverError(w, err)
			return
		}
	}
	if !hasSkillMD {
		s.jsonErrorCode(w, http.StatusBadRequest, ErrCodeValidationFailed, "SKILL.md is required")
		return
	}
	managed := false
	if body.Managed != nil {
		managed = *body.Managed
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sourceType := firstNonEmpty(body.SourceType, "agent")
	source := firstNonEmpty(body.Source, fmt.Sprintf("agent:%s/%s", principal.Project, principal.Agent))
	version := firstNonEmpty(skillVersionFromRef(strings.TrimSpace(body.SourceRef)), skillVersionFromRef(now))
	meta := entity.Skill{
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Source:      source,
		SourceType:  sourceType,
		SourceRef:   strings.TrimSpace(body.SourceRef),
		Version:     version,
		Managed:     managed,
		Dirty:       !managed,
		InstalledAt: now,
		UpdatedAt:   now,
	}
	if err := writeSkillRegistryMeta(dst, meta); err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.snapshotSkillPackage(dst, meta); err != nil {
		s.serverError(w, err)
		return
	}
	s.auditLog(auditLogInput{
		WorkspaceID:  principal.WorkspaceID,
		ActorType:    "agent",
		ActorID:      fmt.Sprintf("%s/%s", principal.Project, principal.Agent),
		Action:       "skill.publish",
		ResourceType: "skill",
		ResourceID:   name,
		Summary:      "Skill published by agent",
		After:        map[string]any{"name": name, "source": source, "sourceType": sourceType},
		Request:      r,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skill": name})
}

func normalizeUploadedSkillContent(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return content
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return content
	}
	return strings.TrimSpace(strings.TrimPrefix(rest[idx+4:], "\n"))
}

func (s *Server) skillRegistryMeta(name string) entity.Skill {
	path := filepath.Join(s.st.SkillDir(name), "skill.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return entity.Skill{}
	}
	var meta entity.Skill
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return entity.Skill{}
	}
	return meta
}

func writeSkillRegistryMeta(skillDir string, meta entity.Skill) error {
	if strings.TrimSpace(meta.Name) == "" {
		return nil
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(&meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(skillDir, "skill.yaml"), raw, 0o644)
}

func (s *Server) snapshotSkillPackage(skillDir string, meta entity.Skill) error {
	name := strings.TrimSpace(meta.Name)
	version := strings.TrimSpace(meta.Version)
	if name == "" || version == "" {
		return nil
	}
	dst := s.skillPackageDir(name, version)
	if dst == "" {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return copyDir(skillDir, dst)
}

func (s *Server) listSkillRegistryPackages() ([]skillPackageRow, error) {
	root := s.skillRegistryRoot()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []skillPackageRow
	for _, skillEntry := range entries {
		if !skillEntry.IsDir() {
			continue
		}
		name := skillEntry.Name()
		versionEntries, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, versionEntry := range versionEntries {
			if !versionEntry.IsDir() {
				continue
			}
			version := versionEntry.Name()
			dir := filepath.Join(root, name, version)
			meta := readSkillMetaFromDir(dir)
			if meta.Name == "" {
				meta.Name = name
			}
			if meta.Version == "" {
				meta.Version = version
			}
			out = append(out, skillPackageRow{
				Name:        meta.Name,
				Description: meta.Description,
				Source:      meta.Source,
				SourceType:  meta.SourceType,
				SourceRef:   meta.SourceRef,
				Version:     meta.Version,
				Managed:     meta.Managed,
				Dirty:       meta.Dirty,
				InstalledAt: meta.InstalledAt,
				UpdatedAt:   meta.UpdatedAt,
				Path:        dir,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.ToLower(out[i].Name) == strings.ToLower(out[j].Name) {
			return out[i].Version > out[j].Version
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func readSkillMetaFromDir(dir string) entity.Skill {
	raw, err := os.ReadFile(filepath.Join(dir, "skill.yaml"))
	if err == nil {
		var meta entity.Skill
		if yaml.Unmarshal(raw, &meta) == nil {
			return meta
		}
	}
	sk, _, err := parseSkillMDForInstall(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return entity.Skill{}
	}
	return sk
}

func parseRegistrySkillSource(source string) (string, string, bool) {
	source = strings.TrimSpace(source)
	source = strings.TrimPrefix(source, "registry://")
	source = strings.TrimPrefix(source, "registry:")
	if source == "" {
		return "", "", false
	}
	name, version, ok := strings.Cut(source, "@")
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	return name, version, ok && name != "" && version != ""
}

func (s *Server) installSkillPackageReference(packageName, version, alias string) error {
	packageName = safeSkillPackagePathPart(packageName)
	version = safeSkillPackagePathPart(version)
	if packageName == "" || version == "" {
		return fmt.Errorf("invalid registry package reference")
	}
	src := s.skillPackageDir(packageName, version)
	if info, err := os.Stat(src); err != nil || !info.IsDir() {
		return errs.NotFound("skill package", packageName+"@"+version)
	}
	meta := readSkillMetaFromDir(src)
	if meta.Name == "" {
		meta.Name = packageName
	}
	if meta.Version == "" {
		meta.Version = version
	}
	name := firstNonEmpty(strings.TrimSpace(alias), meta.Name, packageName)
	if err := validateWorkspaceObjectName("skill", name); err != nil {
		return err
	}
	dst := s.st.SkillDir(name)
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	// Some filesystems or deployment environments disallow symlinks; fall back
	// to a managed copy while keeping the package metadata.
	if err := copyDir(src, dst); err != nil {
		return err
	}
	return writeSkillRegistryMeta(dst, meta)
}

func (s *Server) ensureSkillWritableCopy(name string) error {
	dir := s.st.SkillDir(name)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	target, err := os.Readlink(dir)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dir), target)
	}
	tmp := dir + ".forking"
	_ = os.RemoveAll(tmp)
	if err := copyDir(target, tmp); err != nil {
		return err
	}
	if err := os.Remove(dir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	meta := s.skillRegistryMeta(name)
	if meta.Name == "" {
		meta.Name = name
	}
	meta.Source = "registry:" + meta.Name + "@" + meta.Version
	meta.SourceType = "workspace"
	meta.Managed = false
	meta.Dirty = true
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeSkillRegistryMeta(dir, meta)
}

func (s *Server) skillPackageDir(name, version string) string {
	name = safeSkillPackagePathPart(name)
	version = safeSkillPackagePathPart(version)
	if name == "" || version == "" {
		return ""
	}
	return filepath.Join(s.skillRegistryRoot(), name, version)
}

func (s *Server) skillPackageDirForMeta(fallbackName string, meta entity.Skill) string {
	name := fallbackName
	version := meta.Version
	if sourceName, sourceVersion, ok := parseRegistrySkillSource(meta.Source); ok {
		name = sourceName
		if version == "" {
			version = sourceVersion
		}
	}
	if meta.Name != "" && meta.SourceType != "workspace" {
		name = meta.Name
	}
	return s.skillPackageDir(name, version)
}

func (s *Server) skillRegistryRoot() string {
	root := s.root
	if root == "" && s.st != nil {
		root = s.st.Root()
	}
	dataRoot := filepath.Dir(root)
	if filepath.Base(root) == ".multigent" {
		dataRoot = filepath.Dir(root)
	}
	return filepath.Join(dataRoot, ".multigent", "skill-registry")
}

func safeSkillPackagePathPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, ":", "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.Trim(value, ".-")
	return value
}

func skillVersionFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if len(ref) >= 12 && isHexish(ref[:12]) {
		return ref[:12]
	}
	ref = strings.ReplaceAll(ref, ":", "")
	ref = strings.ReplaceAll(ref, "-", "")
	ref = strings.ReplaceAll(ref, "T", "")
	ref = strings.ReplaceAll(ref, "Z", "")
	ref = strings.ReplaceAll(ref, "+", "")
	ref = strings.ReplaceAll(ref, ".", "")
	ref = safeSkillPackagePathPart(ref)
	if ref == "" {
		return ""
	}
	if len(ref) > 48 {
		return ref[:48]
	}
	return ref
}

func isHexish(value string) bool {
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func skillFileTree(root string, includeContent bool) ([]skillFileEntry, error) {
	walkRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		if info, err := os.Stat(resolved); err == nil && info.IsDir() {
			walkRoot = resolved
		}
	}
	var files []skillFileEntry
	err := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := skillFileEntry{
			Path: rel,
			Size: info.Size(),
			Mode: info.Mode().String(),
		}
		if includeContent && info.Size() <= 512*1024 {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if isLikelyText(raw) {
				entry.Content = string(raw)
				entry.Encoding = "text"
			} else {
				entry.Content = base64.StdEncoding.EncodeToString(raw)
				entry.Encoding = "base64"
			}
		}
		files = append(files, entry)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func isLikelyText(raw []byte) bool {
	for _, b := range raw {
		if b == 0 {
			return false
		}
	}
	return true
}

func decodeSkillFileContent(f skillFileEntry) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(f.Encoding)) {
	case "", "text":
		return []byte(f.Content), nil
	case "base64":
		raw, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 content for %s", f.Path)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("unsupported encoding %q for %s", f.Encoding, f.Path)
	}
}

func cleanSkillRelativePath(path string) (string, error) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("invalid skill file path %q", path)
	}
	clean := filepath.Clean(path)
	clean = filepath.ToSlash(clean)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid skill file path %q", path)
	}
	if strings.HasPrefix(clean, ".git/") || strings.Contains(clean, "/.git/") {
		return "", fmt.Errorf("git metadata is not allowed in skill files")
	}
	return clean, nil
}

func looksLikeGitSource(source string) bool {
	return strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git") ||
		(strings.Count(source, "/") == 1 && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "/"))
}

func cloneSkillSource(source, dst string) error {
	cloneURL := source
	if strings.Count(source, "/") == 1 && !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		cloneURL = "https://github.com/" + source + ".git"
	}
	cmd := exec.Command("git", "clone", "--depth", "1", cloneURL, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitHead(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func resolveSkillInstallSource(srcDir, overrideName, overrideDescription string) (string, string, string, error) {
	candidates := []string{srcDir}
	_ = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == srcDir {
			return nil
		}
		if d.Name() == ".git" || d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			candidates = append(candidates, path)
		}
		return nil
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i] == srcDir {
			return true
		}
		if candidates[j] == srcDir {
			return false
		}
		return len(candidates[i]) < len(candidates[j])
	})
	for _, candidate := range candidates {
		skillMD := filepath.Join(candidate, "SKILL.md")
		if _, err := os.Stat(skillMD); err != nil {
			continue
		}
		sk, _, err := parseSkillMDForInstall(skillMD)
		if err != nil {
			return "", "", "", err
		}
		name := firstNonEmpty(strings.TrimSpace(overrideName), strings.TrimSpace(sk.Name), filepath.Base(candidate))
		description := firstNonEmpty(strings.TrimSpace(overrideDescription), strings.TrimSpace(sk.Description))
		return name, description, candidate, nil
	}
	return "", "", "", fmt.Errorf("no SKILL.md found in source")
}

func parseSkillMDForInstall(path string) (entity.Skill, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return entity.Skill{}, "", err
	}
	content := string(raw)
	if !strings.HasPrefix(content, "---") {
		return entity.Skill{Name: strings.TrimSuffix(filepath.Base(filepath.Dir(path)), filepath.Ext(path))}, content, nil
	}
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return entity.Skill{}, content, nil
	}
	var sk entity.Skill
	if err := yaml.Unmarshal([]byte(rest[:idx]), &sk); err != nil {
		return entity.Skill{}, "", err
	}
	return sk, strings.TrimPrefix(rest[idx+4:], "\n"), nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		clean, err := cleanSkillRelativePath(rel)
		if err != nil {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out := filepath.Join(dst, clean)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		mode := info.Mode()
		if mode == 0 {
			mode = 0o644
		}
		return os.WriteFile(out, raw, mode)
	})
}

func (s *Server) handlePutSkillPrompt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	sk, err := s.st.Skill(name)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "skill not found")
			return
		}
		s.serverError(w, err)
		return
	}

	var body promptSaveBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", sk.Name))
	if sk.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", sk.Description))
	}
	sb.WriteString("---\n")
	if body.Content != "" {
		sb.WriteString("\n")
		sb.WriteString(body.Content)
		if !strings.HasSuffix(body.Content, "\n") {
			sb.WriteString("\n")
		}
	}

	if err := s.ensureSkillWritableCopy(name); err != nil {
		s.serverError(w, err)
		return
	}
	skillMD := filepath.Join(s.st.SkillDir(name), "SKILL.md")
	if err := os.WriteFile(skillMD, []byte(sb.String()), 0o644); err != nil {
		s.serverError(w, err)
		return
	}
	meta := s.skillRegistryMeta(name)
	if meta.Name == "" {
		meta.Name = name
	}
	meta.Description = sk.Description
	meta.Dirty = true
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = writeSkillRegistryMeta(s.st.SkillDir(name), meta)
	s.markPlaybookObjectCustomized(r, "skill", "", name)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── Role / Team skill binding ─────────────────────────────────────────────────

type skillBindBody struct {
	Team   string `json:"team"`
	Role   string `json:"role"`
	Skill  string `json:"skill"`
	Action string `json:"action"` // "add" or "remove"
}

func (s *Server) handlePostRoleSkillBind(w http.ResponseWriter, r *http.Request) {
	var body skillBindBody
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	teamPath := strings.TrimSpace(body.Team)
	roleName := strings.TrimSpace(body.Role)
	skillName := strings.TrimSpace(body.Skill)
	action := strings.TrimSpace(body.Action)

	if teamPath == "" || roleName == "" || skillName == "" {
		s.jsonError(w, http.StatusBadRequest, "team, role, and skill are required")
		return
	}
	if action != "add" && action != "remove" {
		s.jsonError(w, http.StatusBadRequest, "action must be add or remove")
		return
	}

	role, err := s.st.Role(teamPath, roleName)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "role not found")
			return
		}
		s.serverError(w, err)
		return
	}

	if action == "add" {
		if _, err := s.st.Skill(skillName); err != nil {
			s.jsonError(w, http.StatusBadRequest, "skill not found: "+skillName)
			return
		}
		for _, sk := range role.Skills {
			if sk == skillName {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": role.Skills})
				return
			}
		}
		role.Skills = append(role.Skills, skillName)
	} else {
		filtered := make([]string, 0, len(role.Skills))
		for _, sk := range role.Skills {
			if sk != skillName {
				filtered = append(filtered, sk)
			}
		}
		role.Skills = filtered
	}

	if err := s.st.SaveRole(teamPath, roleName, role); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "role", teamPath, roleName)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": role.Skills})
}

func (s *Server) handlePostTeamSkillBind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Team   string `json:"team"`
		Skill  string `json:"skill"`
		Action string `json:"action"`
	}
	if err := s.readJSON(w, r, &body); err != nil {
		s.jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	teamPath := strings.TrimSpace(body.Team)
	skillName := strings.TrimSpace(body.Skill)
	action := strings.TrimSpace(body.Action)

	if teamPath == "" || skillName == "" {
		s.jsonError(w, http.StatusBadRequest, "team and skill are required")
		return
	}
	if action != "add" && action != "remove" {
		s.jsonError(w, http.StatusBadRequest, "action must be add or remove")
		return
	}

	team, err := s.st.Team(teamPath)
	if err != nil {
		if isNotFoundErr(err) {
			s.jsonError(w, http.StatusNotFound, "team not found")
			return
		}
		s.serverError(w, err)
		return
	}

	if action == "add" {
		if _, err := s.st.Skill(skillName); err != nil {
			s.jsonError(w, http.StatusBadRequest, "skill not found: "+skillName)
			return
		}
		for _, sk := range team.Skills {
			if sk == skillName {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": team.Skills})
				return
			}
		}
		team.Skills = append(team.Skills, skillName)
	} else {
		filtered := make([]string, 0, len(team.Skills))
		for _, sk := range team.Skills {
			if sk != skillName {
				filtered = append(filtered, sk)
			}
		}
		team.Skills = filtered
	}

	if err := s.st.SaveTeam(teamPath, team); err != nil {
		s.serverError(w, err)
		return
	}
	s.markPlaybookObjectCustomized(r, "team", "", teamPath)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "skills": team.Skills})
}
