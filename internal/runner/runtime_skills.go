package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/multigent/multigent/internal/ctxbuild"
	"github.com/multigent/multigent/internal/entity"
	"github.com/multigent/multigent/internal/formatter"
)

// runtimeSkillsBundleDir holds the skills materialized for a direct-host run.
// It is passed to Claude Code with --add-dir, so it is found regardless of the
// process working directory or HOME the runtime provider uses.
const runtimeSkillsBundleDir = "skills-bundle"

// runtimeAddDirs returns the agent's configured add-dirs plus, for direct-host
// Claude Code runs, the directory holding its merged skills.
func (r *Runner) runtimeAddDirs(project, agentName, agentDir string, meta *entity.AgentMeta) []string {
	addDirs := append([]string(nil), meta.AddDirs...)
	if bundle := r.runtimeSkillsBundle(project, agentName, agentDir, meta); bundle != "" {
		addDirs = append(addDirs, bundle)
	}
	return addDirs
}

// runtimeSkillsBundle materializes the skills an agent is entitled to
// (defaults, team, role and Agent Worker skills) for a direct-host Claude Code
// run. Multigent 2.x resolves context at run time instead of keeping per-agent
// context directories in sync, so without this step role and worker skills
// never reach the run.
func (r *Runner) runtimeSkillsBundle(project, agentName, agentDir string, meta *entity.AgentMeta) string {
	if r == nil || r.agentStore == nil || meta == nil || entity.NormaliseModel(meta.Model) != entity.ModelClaudeCode {
		return ""
	}
	if meta.Sandbox != nil && meta.Sandbox.Provider != entity.SandboxNone {
		return "" // container runs only see mounted paths; their image owns the skills
	}
	mc, err := ctxbuild.NewBuilder(r.agentStore).BuildForAgent(project, agentName, meta.Team, meta.Role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "multigent: runtime skills unavailable for %s/%s: %v\n", project, agentName, err)
		return ""
	}
	if mc == nil || len(mc.Skills) == 0 {
		return ""
	}
	dir := filepath.Join(agentDir, ".multigent", runtimeSkillsBundleDir)
	if err := formatter.WriteClaudeCodeSkills(mc.Skills, dir); err != nil {
		fmt.Fprintf(os.Stderr, "multigent: write runtime skills for %s/%s: %v\n", project, agentName, err)
		return ""
	}
	return dir
}
