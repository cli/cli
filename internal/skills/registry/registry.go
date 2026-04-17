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
var Agents = []AgentHost{
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
		ID:         "gemini",
		Name:       "Gemini CLI",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".gemini/skills",
	},
	{
		ID:         "junie",
		Name:       "Junie",
		ProjectDir: ".junie/skills",
		UserDir:    ".junie/skills",
	},
	{
		ID:         "antigravity",
		Name:       "Antigravity",
		ProjectDir: sharedProjectSkillsDir,
		UserDir:    ".gemini/antigravity/skills",
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
