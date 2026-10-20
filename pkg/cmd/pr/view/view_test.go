package view

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestViewRunPipedSkipsUnusedPreloads(t *testing.T) {
	tests := []struct {
		name          string
		terminal      bool
		comments      bool
		jsonFields    []string
		wantPreloads  bool
		wantOutput    string
	}{
		{
			name: "piped default",
			wantOutput: "title:\tExample\nstate:\tOPEN\nauthor:\toctocat\nlabels:\t\nassignees:\t\nreviewers:\treviewer (Approved)\nprojects:\t\nmilestone:\t\nnumber:\t1\nurl:\thttps://github.com/owner/repo/pull/1\nadditions:\t0\ndeletions:\t0\nauto-merge:\tdisabled\n--\nBody\n",
		},
		{
			name: "terminal default",
			terminal: true,
			wantPreloads: true,
			wantOutput: "View this pull request on GitHub: https://github.com/owner/repo/pull/1",
		},
		{
			name: "piped comments",
			comments: true,
			wantPreloads: true,
			wantOutput: "Paginated comment",
		},
		{
			name: "terminal comments",
			terminal: true,
			comments: true,
			wantPreloads: true,
			wantOutput: "Paginated comment",
		},
		{
			name: "json comments and checks",
			jsonFields: []string{"comments", "statusCheckRollup"},
			wantPreloads: true,
			wantOutput: "Paginated comment",
		},
		{
			name: "json title",
			jsonFields: []string{"title"},
			wantOutput: "{\"title\":\"Example\"}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.terminal)
			registry := &httpmock.Registry{}
			registry.Register(httpmock.GraphQL("PullRequestByNumber"), httpmock.GraphQLQuery(`{"data":{"repository":{"pullRequest":{"id":"PR_1","number":1,"title":"Example","state":"OPEN","body":"Body","author":{"login":"octocat"},"url":"https://github.com/owner/repo/pull/1","reviews":{"nodes":[{"author":{"login":"reviewer"},"state":"APPROVED"}]},"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"comments-page-1"}},"statusCheckRollup":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":true,"endCursor":"checks-page-1"}}}}}]}}}}}`, func(query string, _ map[string]any) {
				require.Equal(t, tt.wantPreloads, strings.Contains(query, "comments("), "initial comments selection")
				require.Equal(t, tt.wantPreloads, strings.Contains(query, "statusCheckRollup"), "initial checks selection")
				if tt.jsonFields == nil {
					require.Contains(t, query, "reviews(")
				}
				if !tt.terminal && !tt.comments && tt.jsonFields == nil {
					expectedFields := []string{
						"url", "number", "title", "state", "body", "author", "autoMergeRequest",
						"isDraft", "additions", "deletions", "reviewRequests", "reviews", "assignees",
						"labels", "milestone", "id",
					}
					require.Contains(t, query, "{"+api.PullRequestGraphQL(expectedFields)+"}")
				}
			}))
			if tt.wantPreloads {
				registry.Register(httpmock.GraphQL("CommentsForPullRequest"), httpmock.StringResponse(`{"data":{"node":{"comments":{"nodes":[{"author":{"login":"octocat"},"body":"Paginated comment","createdAt":"2026-01-01T00:00:00Z"}],"pageInfo":{"hasNextPage":false}}}}}`))
				registry.Register(httpmock.GraphQL("PullRequestStatusChecks"), httpmock.StringResponse(`{"data":{"node":{"statusCheckRollup":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}]}}}}`))
			}
			if tt.jsonFields == nil {
				registry.Register(httpmock.GraphQL("PullRequestProjectItems"), httpmock.StringResponse(`{"data":{"repository":{"pullRequest":{"projectItems":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`))
			}
			factory := &cmdutil.Factory{
				IOStreams: ios,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: registry}, nil
				},
			}
			opts := &ViewOptions{
				IO: ios,
				Finder: shared.NewFinder(factory),
				Detector: &fd.DisabledDetectorMock{},
				SelectorArg: "https://github.com/owner/repo/pull/1",
				Comments: tt.comments,
				Now: func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) },
			}
			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}
			require.NoError(t, viewRun(opts))
			if tt.wantPreloads {
				require.Contains(t, stdout.String(), tt.wantOutput)
			} else {
				require.Equal(t, tt.wantOutput, stdout.String())
			}
			registry.Verify(t)
		})
	}
}
