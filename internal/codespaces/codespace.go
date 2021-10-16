package codespaces

import (
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
)

type codespace struct {
	*api.Codespace
}

// displayName returns the repository nwo and branch.
// If includeName is true, the name of the codespace is included.
// If includeGitStatus is true, the branch will include a star if
// the codespace has unsaved changes.
func (c codespace) displayName(includeName, includeGitStatus bool) string {
	branch := c.GitStatus.Ref
	if includeGitStatus {
		branch = c.branchWithGitStatus()
	}

	if includeName {
		return fmt.Sprintf(
			"%s: %s [%s]", c.Repository.FullName, branch, c.Name,
		)
	}
	return c.Repository.FullName + ": " + branch
}

// gitStatusDirty represents an unsaved changes status.
const gitStatusDirty = "*"

// branchWithGitStatus returns the branch with a star
// if the branch is currently being worked on.
func (c codespace) branchWithGitStatus() string {
	if c.hasUnsavedChanges() {
		return c.GitStatus.Ref + gitStatusDirty
	}

	return c.GitStatus.Ref
}

// hasUnsavedChanges returns whether the environment has
// unsaved changes.
func (c codespace) hasUnsavedChanges() bool {
	return c.GitStatus.HasUncommitedChanges || c.GitStatus.HasUnpushedChanges
}

// running returns whether the codespace environment is running.
func (c codespace) running() bool {
	return c.State == api.CodespaceStateAvailable
}

// connectionReady returns whether the codespace environment is ready to
// be connected to.
func (c codespace) connectionReady() bool {
	return c.Connection.SessionID != "" &&
		c.Connection.SessionToken != "" &&
		c.Connection.RelayEndpoint != "" &&
		c.Connection.RelaySAS != "" &&
		c.running()
}
