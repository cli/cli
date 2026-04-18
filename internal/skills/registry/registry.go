package registry

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
)

// AgentHost represents an AI agent that can use skills.
type AgentHost struct {
	// ID is the canonical identifier for this agent host.
	ID string
	// Name is the human-readable display name.
	Name string
	// ProjectDir is the relative path within a project for skills.
	ProjectDir string
	// UserDir is the relative path within the user's home directory for skills.
	UserDir string
}

// Scope determines where skills are installed.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"

	DefaultAgentID = "github-copilot"

	sharedProjectSkillsDir = ".agents/skills"
)

// Agents contains all known agent hosts.
//
// The slice is ordered so that the most widely used agents appear first,
// followed by the rest in alphabetical order. This order is used for
// interactive selection, help output, and flag enum suggestions.
//
// Agents sharing a ProjectDir (such as the shared .agents/skills directory)
// install skills to the same project-scope location, so selecting multiple
// such agents writes each skill only once.
var Agents = []AgentHost{
	// Popular agents, listed first for discoverability.
	{
		ID:         "github-copilot",
		Name:       "GitHub Copilot",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".copilot/skills",
	},
	{
		ID:         "claude-code",
		Name:       "Claude Code",
		ProjectDir: ".claude/skills",
		UserDir:    ".claude/skills",
	},
	{
		ID:         "cursor",
		Name:       "Cursor",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".cursor/skills",
	},
	{
		ID:         "codex",
		Name:       "Codex",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".codex/skills",
	},
	{
		ID:         "gemini-cli",
		Name:       "Gemini CLI",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".gemini/skills",
	},
	{
		ID:         "antigravity",
		Name:       "Antigravity",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".gemini/antigravity/skills",
	},

	// All other supported agents, alphabetical by ID.
	{
		ID:         "adal",
		Name:       "AdaL",
		ProjectDir: ".adal/skills",
		UserDir:    ".adal/skills",
	},
	{
		ID:         "amp",
		Name:       "Amp",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".config/agents/skills",
	},
	{
		ID:         "augment",
		Name:       "Augment",
		ProjectDir: ".augment/skills",
		UserDir:    ".augment/skills",
	},
	{
		ID:         "bob",
		Name:       "IBM Bob",
		ProjectDir: ".bob/skills",
		UserDir:    ".bob/skills",
	},
	{
		ID:         "cline",
		Name:       "Cline",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".agents/skills",
	},
	{
		ID:         "codebuddy",
		Name:       "CodeBuddy",
		ProjectDir: ".codebuddy/skills",
		UserDir:    ".codebuddy/skills",
	},
	{
		ID:         "command-code",
		Name:       "Command Code",
		ProjectDir: ".commandcode/skills",
		UserDir:    ".commandcode/skills",
	},
	{
		ID:         "continue",
		Name:       "Continue",
		ProjectDir: ".continue/skills",
		UserDir:    ".continue/skills",
	},
	{
		ID:         "cortex",
		Name:       "Cortex Code",
		ProjectDir: ".cortex/skills",
		UserDir:    ".snowflake/cortex/skills",
	},
	{
		ID:         "crush",
		Name:       "Crush",
		ProjectDir: ".crush/skills",
		UserDir:    ".config/crush/skills",
	},
	{
		ID:         "deepagents",
		Name:       "Deep Agents",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".deepagents/agent/skills",
	},
	{
		ID:         "droid",
		Name:       "Droid",
		ProjectDir: ".factory/skills",
		UserDir:    ".factory/skills",
	},
	{
		ID:         "firebender",
		Name:       "Firebender",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".firebender/skills",
	},
	{
		ID:         "goose",
		Name:       "Goose",
		ProjectDir: ".goose/skills",
		UserDir:    ".config/goose/skills",
	},
	{
		ID:         "iflow-cli",
		Name:       "iFlow CLI",
		ProjectDir: ".iflow/skills",
		UserDir:    ".iflow/skills",
	},
	{
		ID:         "junie",
		Name:       "Junie",
		ProjectDir: ".junie/skills",
		UserDir:    ".junie/skills",
	},
	{
		ID:         "kilo",
		Name:       "Kilo Code",
		ProjectDir: ".kilocode/skills",
		UserDir:    ".kilocode/skills",
	},
	{
		ID:         "kimi-cli",
		Name:       "Kimi Code CLI",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".config/agents/skills",
	},
	{
		ID:         "kiro-cli",
		Name:       "Kiro CLI",
		ProjectDir: ".kiro/skills",
		UserDir:    ".kiro/skills",
	},
	{
		ID:         "kode",
		Name:       "Kode",
		ProjectDir: ".kode/skills",
		UserDir:    ".kode/skills",
	},
	{
		ID:         "mcpjam",
		Name:       "MCPJam",
		ProjectDir: ".mcpjam/skills",
		UserDir:    ".mcpjam/skills",
	},
	{
		ID:         "mistral-vibe",
		Name:       "Mistral Vibe",
		ProjectDir: ".vibe/skills",
		UserDir:    ".vibe/skills",
	},
	{
		ID:         "mux",
		Name:       "Mux",
		ProjectDir: ".mux/skills",
		UserDir:    ".mux/skills",
	},
	{
		ID:         "neovate",
		Name:       "Neovate",
		ProjectDir: ".neovate/skills",
		UserDir:    ".neovate/skills",
	},
	{
		ID:         "openclaw",
		Name:       "OpenClaw",
		ProjectDir: "skills",
		UserDir:    ".openclaw/skills",
	},
	{
		ID:         "opencode",
		Name:       "OpenCode",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".config/opencode/skills",
	},
	{
		ID:         "openhands",
		Name:       "OpenHands",
		ProjectDir: ".openhands/skills",
		UserDir:    ".openhands/skills",
	},
	{
		ID:         "pi",
		Name:       "Pi",
		ProjectDir: ".pi/skills",
		UserDir:    ".pi/agent/skills",
	},
	{
		ID:         "pochi",
		Name:       "Pochi",
		ProjectDir: ".pochi/skills",
		UserDir:    ".pochi/skills",
	},
	{
		ID:         "qoder",
		Name:       "Qoder",
		ProjectDir: ".qoder/skills",
		UserDir:    ".qoder/skills",
	},
	{
		ID:         "qwen-code",
		Name:       "Qwen Code",
		ProjectDir: ".qwen/skills",
		UserDir:    ".qwen/skills",
	},
	{
		ID:         "replit",
		Name:       "Replit",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".config/agents/skills",
	},
	{
		ID:         "roo",
		Name:       "Roo Code",
		ProjectDir: ".roo/skills",
		UserDir:    ".roo/skills",
	},
	{
		ID:         "trae",
		Name:       "Trae",
		ProjectDir: ".trae/skills",
		UserDir:    ".trae/skills",
	},
	{
		ID:         "trae-cn",
		Name:       "Trae CN",
		ProjectDir: ".trae/skills",
		UserDir:    ".trae-cn/skills",
	},
	{
		ID:         "universal",
		Name:       "Universal",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".config/agents/skills",
	},
	{
		ID:         "warp",
		Name:       "Warp",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".agents/skills",
	},
	{
		ID:         "windsurf",
		Name:       "Windsurf",
		ProjectDir: ".windsurf/skills",
		UserDir:    ".codeium/windsurf/skills",
	},
	{
		ID:         "zencoder",
		Name:       "Zencoder",
		ProjectDir: ".zencoder/skills",
		UserDir:    ".zencoder/skills",
	},
}

// FindByID returns the agent host with the given ID, or an error if not found.
func FindByID(id string) (*AgentHost, error) {
	for i := range Agents {
		if Agents[i].ID == id {
			return &Agents[i], nil
		}
	}
	return nil, fmt.Errorf("unknown agent %q, valid agents: %s", id, ValidAgentIDs())
}

// ValidAgentIDs returns a comma-separated list of valid agent IDs.
func ValidAgentIDs() string {
	return strings.Join(AgentIDs(), ", ")
}

// AgentIDs returns the IDs of all known agents as a slice.
func AgentIDs() []string {
	ids := make([]string, len(Agents))
	for i, h := range Agents {
		ids[i] = h.ID
	}
	return ids
}

// AgentHelpList returns a newline-separated bulleted list of agents for help text.
func AgentHelpList() string {
	lines := make([]string, len(Agents))
	for i, h := range Agents {
		lines[i] = fmt.Sprintf("  - %s (%s)", h.Name, h.ID)
	}
	return strings.Join(lines, "\n")
}

// AgentNames returns the display names of all agents for prompting.
func AgentNames() []string {
	names := make([]string, len(Agents))
	for i, h := range Agents {
		names[i] = h.Name
	}
	return names
}

// UniqueProjectDirs returns the deduplicated set of project-scope skill
// directories from the Agents list, preserving insertion order.
func UniqueProjectDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	for _, h := range Agents {
		if !seen[h.ProjectDir] {
			seen[h.ProjectDir] = true
			dirs = append(dirs, h.ProjectDir)
		}
	}
	return dirs
}

// InstallDir resolves the absolute installation directory for an agent host and scope.
// For project scope, it uses the provided git root directory so that skills are
// installed at the top level regardless of which subdirectory the user is in.
// Returns an error when gitRoot is empty (not in a git repository).
// For user scope, it uses the home directory.
func (h *AgentHost) InstallDir(scope Scope, gitRoot, homeDir string) (string, error) {
	switch scope {
	case ScopeProject:
		if gitRoot == "" {
			return "", fmt.Errorf("could not determine project root directory")
		}
		return filepath.Join(gitRoot, h.ProjectDir), nil
	case ScopeUser:
		if homeDir == "" {
			return "", fmt.Errorf("could not determine home directory")
		}
		return filepath.Join(homeDir, h.UserDir), nil
	default:
		return "", fmt.Errorf("invalid scope %q", scope)
	}
}

// ScopeLabels returns the display labels for the scope selection prompt.
// If repoName is non-empty, it is included in the project-scope label
// for additional context.
func ScopeLabels(repoName string) []string {
	projectLabel := "Project: install in current repository (recommended)"
	if repoName != "" {
		projectLabel = fmt.Sprintf("Project: %s (recommended)", repoName)
	}
	return []string{
		projectLabel,
		"Global: install in home directory (available everywhere)",
	}
}

// RepoNameFromRemote extracts "owner/repo" from a git remote URL.
func RepoNameFromRemote(remote string) string {
	if remote == "" {
		return ""
	}
	u, err := git.ParseURL(remote)
	if err != nil {
		return ""
	}
	repo, err := ghrepo.FromURL(u)
	if err != nil {
		return ""
	}
	return ghrepo.FullName(repo)
}
