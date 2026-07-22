// Package ctxbuild builds a MergedContext by collecting workspace, team, role,
// project, and skill context. It is the core business logic of multigent and
// has no dependency on any agent-specific format.
package ctxbuild

// MergedContext is the agent-agnostic representation of a fully assembled
// context for one (project, team, role) tuple. The formatter layer consumes
// this and translates it into whatever format a specific agent requires.
type MergedContext struct {
	// Layers holds the ordered prompt sections, from most general to most
	// specific. The order is: workspace → team → role → project.
	Layers []ContextLayer

	// Skills holds deduplicated skills collected from the team and role.
	Skills []SkillDef
}

// ContextLayer is one prompt section with a human-readable source label.
type ContextLayer struct {
	// Source identifies where this content came from, e.g.:
	//   "agency"
	//   "team:engineering"
	//   "role:<team>/<role>"
	//   "project:cc-connect"
	Source string

	// Content is the raw Markdown text of the prompt.md for this layer.
	Content string
}

// SkillFile is a non-documentation file bundled with a skill (e.g. a shell
// script). It is copied verbatim into the agent's skill directory at format
// time so the agent can execute it without any host path dependency.
type SkillFile struct {
	// Name is the bare filename, e.g. "git-push-github.sh".
	Name    string
	Content []byte
}

// SkillDef is a resolved skill with its prompt content ready to embed.
//
// Prompt text may use the placeholder {{SKILL_DIR}} which formatters replace
// with the absolute path of the deployed skill directory so agents can locate
// bundled scripts regardless of where the agent workspace lives.
type SkillDef struct {
	Name        string
	Description string
	// Prompt is the Markdown body of the skill's SKILL.md (after frontmatter).
	Prompt string
	// Files holds any extra files found in the skill's source directory
	// (everything except SKILL.md, prompt.md, and obsolete definition files).
	// Typically shell scripts.
	Files []SkillFile
}
