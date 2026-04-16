// Package shared provides factory functions, field definitions, and display
// helpers used across discussion subcommands.
package shared

import (
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

// DiscussionFields lists all field names available for --json output
// on discussion commands that return a full discussion (e.g. view).
var DiscussionFields = []string{
	"id",
	"number",
	"title",
	"body",
	"url",
	"closed",
	"state",
	"stateReason",
	"author",
	"category",
	"labels",
	"answered",
	"answerChosenAt",
	"answerChosenBy",
	"comments",
	"reactionGroups",
	"createdAt",
	"updatedAt",
	"closedAt",
	"locked",
}

// DiscussionClientFunc returns a factory function that creates a DiscussionClient
// from the given Factory. The returned function is intended to be stored in
// command Options structs and called lazily inside RunE.
func DiscussionClientFunc(f *cmdutil.Factory) func() (client.DiscussionClient, error) {
	return func() (client.DiscussionClient, error) {
		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}
		return client.NewDiscussionClient(httpClient), nil
	}
}
