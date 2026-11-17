package shared

import (
	"fmt"
	"strings"
)

// EventType describes an event type that can be subscribed to.
type EventType struct {
	Name        string
	Description string
}

// EventTypes lists the supported event types.
var EventTypes = []EventType{
	{"com.github.pull_request.review.submitted.v1", "a review was submitted"},
	{"com.github.pull_request.state.changed.v1", "the pull request was merged, closed, reopened, marked ready for review, or converted to draft"},
	{"com.github.pull_request.head.changed.v1", "the head branch changed, for example after a push"},
	{"com.github.pull_request.merge_conflict.v1", "the pull request has a merge conflict"},
	{"com.github.pull_request.check_suite.status.changed.v1", "a check suite on the head commit completed, such as a CI workflow"},
}

// EventTypeDescription returns the description of the named event type, or an empty string if it is unknown.
func EventTypeDescription(name string) string {
	for _, t := range EventTypes {
		if t.Name == name {
			return t.Description
		}
	}
	return ""
}

// EventTypesHelp returns the supported event types as a Markdown list for command help, without a trailing newline.
func EventTypesHelp() string {
	lines := make([]string, len(EventTypes))
	for i, t := range EventTypes {
		lines[i] = fmt.Sprintf("- `%s`: %s", t.Name, t.Description)
	}
	return strings.Join(lines, "\n")
}
