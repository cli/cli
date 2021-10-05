package codespace

import "fmt"

type Codespace struct {
	Name           string      `json:"name"`
	CreatedAt      string      `json:"created_at"`
	LastUsedAt     string      `json:"last_used_at"`
	GUID           string      `json:"guid"`
	Branch         string      `json:"branch"`
	RepositoryName string      `json:"repository_name"`
	RepositoryNWO  string      `json:"repository_nwo"`
	OwnerLogin     string      `json:"owner_login"`
	Environment    Environment `json:"environment"`
}

// DisplayName returns the repository nwo and branch.
// If includeName is true, the name of the codespace is included.
// If includeGitStatus is true, the branch will include a star if
// the codespace has unsaved changes.
func (c *Codespace) DisplayName(includeName, includeGitStatus bool) string {
	branch := c.Branch
	if includeGitStatus {
		branch = c.BranchWithGitStatus()
	}

	if includeName {
		return fmt.Sprintf(
			"%s: %s [%s]", c.RepositoryNWO, branch, c.Name,
		)
	}
	return c.RepositoryNWO + ": " + branch
}

// BranchWithGitStatus returns the branch with a star
// if the branch is currently being worked on.
func (c *Codespace) BranchWithGitStatus() string {
	if c.HasUnsavedChanges() {
		return c.Branch + "*"
	}

	return c.Branch
}

// HasUnsavedChanges returns whether the environment has
// unsaved changes.
func (c *Codespace) HasUnsavedChanges() bool {
	return c.Environment.GitStatus.HasUncommitedChanges || c.Environment.GitStatus.HasUnpushedChanges
}

type Environment struct {
	State      string                `json:"state"`
	Connection EnvironmentConnection `json:"connection"`
	GitStatus  EnvironmentGitStatus  `json:"gitStatus"`
}

type EnvironmentGitStatus struct {
	Ahead                int    `json:"ahead"`
	Behind               int    `json:"behind"`
	Branch               string `json:"branch"`
	Commit               string `json:"commit"`
	HasUnpushedChanges   bool   `json:"hasUnpushedChanges"`
	HasUncommitedChanges bool   `json:"hasUncommitedChanges"`
}

const (
	EnvironmentStateAvailable = "Available"
)

type EnvironmentConnection struct {
	SessionID      string   `json:"sessionId"`
	SessionToken   string   `json:"sessionToken"`
	RelayEndpoint  string   `json:"relayEndpoint"`
	RelaySAS       string   `json:"relaySas"`
	HostPublicKeys []string `json:"hostPublicKeys"`
}
