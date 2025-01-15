package format

import (
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
)

func ProjectState(project queries.Project) string {
	if project.Closed {
		return "closed"
	}
	return "open"
}

func ColorForProjectState(project queries.Project) string {
	if project.Closed {
		return "gray"
	}
	return "green"
}
