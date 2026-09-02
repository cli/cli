package edit

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tmpImage := filepath.Join(t.TempDir(), "shot.png")
	require.NoError(t, os.WriteFile(tmpImage, []byte("the bytes"), 0600))

	tests := []struct {
		name             string
		input            string
		stdin            string
		output           EditOptions
		wantAssetPaths   []string
		expectedBaseRepo ghrepo.Interface
		wantsErr         bool
	}{
		{
			name:  "no argument",
			input: "",
			output: EditOptions{
				SelectorArg: "",
				Interactive: true,
			},
			wantsErr: false,
		},
		{
			name:     "two arguments",
			input:    "1 2",
			output:   EditOptions{},
			wantsErr: true,
		},
		{
			name:  "URL argument",
			input: "https://example.com/cli/cli/pull/23",
			output: EditOptions{
				SelectorArg: "https://example.com/cli/cli/pull/23",
				Interactive: true,
			},
			expectedBaseRepo: ghrepo.NewWithHost("cli", "cli", "example.com"),
			wantsErr:         false,
		},
		{
			name:  "pull request number argument",
			input: "23",
			output: EditOptions{
				SelectorArg: "23",
				Interactive: true,
			},
			wantsErr: false,
		},
		{
			name:  "title flag",
			input: "23 --title test",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "23 --body test",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
						Value:  "test",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from stdin",
			input: "23 --body-file -",
			stdin: "this is on standard input",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
						Value:  "this is on standard input",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "body from file",
			input: fmt.Sprintf("23 --body-file '%s'", tmpFile),
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Body: shared.EditableString{
						Value:  "a body from file",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:     "both body and body-file flags",
			input:    "23 --body foo --body-file bar",
			wantsErr: true,
		},
		{
			name:  "base flag",
			input: "23 --base base-branch-name",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-reviewer flag",
			input: "23 --add-reviewer monalisa,owner/core",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{
						Add:    []string{"monalisa", "owner/core"},
						Edited: true,
					}},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-reviewer flag",
			input: "23 --remove-reviewer monalisa,owner/core",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{
						Remove: []string{"monalisa", "owner/core"},
						Edited: true,
					}},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-assignee flag",
			input: "23 --add-assignee monalisa,hubot",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-assignee flag",
			input: "23 --remove-assignee monalisa,hubot",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Remove: []string{"monalisa", "hubot"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-label flag",
			input: "23 --add-label feature,TODO,bug",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-label flag",
			input: "23 --remove-label feature,TODO,bug",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Labels: shared.EditableSlice{
						Remove: []string{"feature", "TODO", "bug"},
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "add-project flag",
			input: "23 --add-project Cleanup,Roadmap",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-project flag",
			input: "23 --remove-project Cleanup,Roadmap",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Remove: []string{"Cleanup", "Roadmap"},
							Edited: true,
						},
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "milestone flag",
			input: "23 --milestone GA",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:  "remove-milestone flag",
			input: "23 --remove-milestone",
			output: EditOptions{
				SelectorArg: "23",
				Editable: shared.Editable{
					Milestone: shared.EditableString{
						Value:  "",
						Edited: true,
					},
				},
			},
			wantsErr: false,
		},
		{
			name:     "both milestone and remove-milestone flags",
			input:    "23 --milestone foo --remove-milestone",
			wantsErr: true,
		},
		{
			name:  "attach alone runs without prompting",
			input: fmt.Sprintf("23 --attach '%s'", tmpImage),
			output: EditOptions{
				SelectorArg: "23",
				Interactive: false,
			},
			wantAssetPaths: []string{tmpImage},
		},
		{
			name:  "attach with body records the body that replaces the old one",
			input: fmt.Sprintf("23 --body 'a new body' --attach '%s'", tmpImage),
			output: EditOptions{
				SelectorArg: "23",
				Interactive: false,
				Editable: shared.Editable{
					Body: shared.EditableString{
						Value:  "a new body",
						Edited: true,
					},
				},
			},
			wantAssetPaths: []string{tmpImage},
		},
		{
			name:     "attach rejects a missing file",
			input:    "23 --attach ./nope.png",
			wantsErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.Editable, gotOpts.Editable)

			var assetPaths []string
			for _, a := range gotOpts.Assets {
				assetPaths = append(assetPaths, a.Path())
			}
			assert.Equal(t, tt.wantAssetPaths, assetPaths)

			if tt.expectedBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				require.True(
					t,
					ghrepo.IsSame(tt.expectedBaseRepo, baseRepo),
					"expected base repo %+v, got %+v", tt.expectedBaseRepo, baseRepo,
				)
			}
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name      string
		input     *EditOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		attach    []string
		// Empty means the default token, which can upload.
		token string
		// Overrides token, for a row that needs more than one host.
		hostTokens map[string]string
		uploads    []attachments.UploadStub
		// Empty means the row does not care which host was reached.
		wantUploadHost     string
		wantLookupFields   []string
		wantNoLookupFields []string
		stdout             string
		stderr             string
		wantErr            string
	}{
		{
			name: "non-interactive",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: shared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{
						Add:    []string{"OWNER/core", "OWNER/external", "monalisa", "hubot"},
						Remove: []string{"dependabot"},
						Edited: true,
					}},
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Add/Remove doesn't need reviewers/assignees metadata
				// REST API accepts logins and team slugs directly
				mockRepoMetadata(reg, mockRepoMetadataOptions{reviewers: false, teamReviewers: false, assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateApiActors(reg)
				mockRequestReviewsByLogin(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive skip reviewers",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: shared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockRepoMetadata(reg, mockRepoMetadataOptions{assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateApiActors(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive remove all reviewers",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{ // include existing reviewers so removal logic triggers
					URL: "https://github.com/OWNER/REPO/pull/123",
					ReviewRequests: api.ReviewRequests{Nodes: []struct{ RequestedReviewer api.RequestedReviewer }{
						{RequestedReviewer: api.RequestedReviewer{TypeName: "Team", Slug: "core", Organization: struct {
							Login string `json:"login"`
						}{Login: "OWNER"}}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "Team", Slug: "external", Organization: struct {
							Login string `json:"login"`
						}{Login: "OWNER"}}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "monalisa"}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "hubot"}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "dependabot"}},
					}},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
					Body: shared.EditableString{
						Value:  "new body",
						Edited: true,
					},
					Base: shared.EditableString{
						Value:  "base-branch-name",
						Edited: true,
					},
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{
						Default: []string{"OWNER/core", "OWNER/external", "monalisa", "hubot", "dependabot"},
						Remove:  []string{"OWNER/core", "OWNER/external", "monalisa", "hubot", "dependabot"},
						Edited:  true,
					}},
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
					Labels: shared.EditableSlice{
						Add:    []string{"feature", "TODO", "bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"Cleanup", "CleanupV2"},
							Remove: []string{"Roadmap", "RoadmapV2"},
							Edited: true,
						},
					},
					Milestone: shared.EditableString{
						Value:  "GA",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Remove doesn't need reviewers metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{reviewers: false, teamReviewers: false, assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
				mockPullRequestUpdateLabels(reg)
				mockPullRequestUpdateApiActors(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "remove all reviewers sends empty slices to mutation",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
					ReviewRequests: api.ReviewRequests{
						Nodes: []struct{ RequestedReviewer api.RequestedReviewer }{
							{
								RequestedReviewer: api.RequestedReviewer{
									TypeName: "Team",
									Slug:     "core",
									Organization: struct {
										Login string `json:"login"`
									}{Login: "OWNER"},
								},
							},
							{
								RequestedReviewer: api.RequestedReviewer{
									TypeName: "User",
									Login:    "monalisa",
								},
							},
						},
					},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{
						Default: []string{"OWNER/core", "monalisa"},
						Remove:  []string{"OWNER/core", "monalisa"},
						Edited:  true,
					}},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockRepoMetadata(reg, mockRepoMetadataOptions{})
				mockPullRequestUpdate(reg)
				reg.Register(
					httpmock.GraphQL(`mutation RequestReviewsByLogin\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "requestReviewsByLogin": { "clientMutationId": "" } } }`,
						func(inputs map[string]any) {
							// Verify that empty slices are sent to properly clear all reviewer types
							require.Equal(t, []any{}, inputs["userLogins"], "userLogins should be an empty slice")
							require.Equal(t, []any{}, inputs["botLogins"], "botLogins should be an empty slice")
							require.Equal(t, []any{}, inputs["teamSlugs"], "teamSlugs should be an empty slice")
							require.Equal(t, false, inputs["union"], "union should be false for replace mode")
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		// Conditional team fetching cases
		{
			name: "non-interactive add only user reviewers skips team fetch",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder:      shared.NewMockFinder("123", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123"}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{Add: []string{"monalisa", "hubot"}, Edited: true}},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Add/Remove doesn't need reviewer metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{})
				// explicitly assert that no OrganizationTeamList query occurs
				reg.Exclude(t, httpmock.GraphQL(`query OrganizationTeamList\b`))
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive add contains team reviewers skips team fetch",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder:      shared.NewMockFinder("123", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123"}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{Add: []string{"monalisa", "OWNER/core"}, Edited: true}},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Add/Remove doesn't need reviewer metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{})
				// explicitly assert that no OrganizationTeamList query occurs
				reg.Exclude(t, httpmock.GraphQL(`query OrganizationTeamList\b`))
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive reviewers remove contains team skips team fetch",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123", ReviewRequests: api.ReviewRequests{Nodes: []struct{ RequestedReviewer api.RequestedReviewer }{
					{RequestedReviewer: api.RequestedReviewer{TypeName: "Team", Slug: "core", Organization: struct {
						Login string `json:"login"`
					}{Login: "OWNER"}}},
					{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "monalisa"}},
				}}}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{Remove: []string{"monalisa", "OWNER/core"}, Edited: true}},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Add/Remove doesn't need reviewer metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{})
				// explicitly assert that no OrganizationTeamList query occurs
				reg.Exclude(t, httpmock.GraphQL(`query OrganizationTeamList\b`))
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive mutate reviewers with no change to existing team reviewers skips team fetch",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder:      shared.NewMockFinder("123", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123"}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Reviewers: shared.EditableReviewers{EditableSlice: shared.EditableSlice{Add: []string{"monalisa"}, Remove: []string{"hubot"}, Default: []string{"OWNER/core"}, Edited: true}},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Non-interactive with Add/Remove doesn't need reviewer metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{})
				// explicitly assert that no OrganizationTeamList query occurs
				reg.Exclude(t, httpmock.GraphQL(`query OrganizationTeamList\b`))
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Title.Edited = true
						e.Body.Edited = true
						e.Reviewers.Edited = true
						e.Assignees.Edited = true
						e.Labels.Edited = true
						e.Projects.Edited = true
						e.Milestone.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						e.Title.Value = "new title"
						e.Body.Value = "new body"
						e.Reviewers.Value = []string{"monalisa", "hubot", "OWNER/core", "OWNER/external"}
						e.Assignees.Value = []string{"monalisa", "hubot"}
						// Populate metadata to simulate what searchFunc would do during prompting
						e.Metadata.AssignableActors = []api.AssignableActor{
							api.NewAssignableBot("HUBOTID", "hubot"),
							api.NewAssignableUser("MONAID", "monalisa", "Mona Display Name"),
						}
						// Populate team metadata for reviewer search
						e.Metadata.Teams = []api.OrgTeam{
							{ID: "COREID", Slug: "core"},
							{ID: "EXTERNALID", Slug: "external"},
						}
						e.Labels.Value = []string{"feature", "TODO", "bug"}
						e.Labels.Add = []string{"feature", "TODO", "bug"}
						e.Labels.Remove = []string{"docs"}
						e.Projects.Value = []string{"Cleanup", "CleanupV2"}
						e.Milestone.Value = "GA"
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// With search functions enabled, we don't fetch reviewers/assignees metadata
				// (searchFunc handles dynamic fetching, metadata populated in test mock)
				mockRepoMetadata(reg, mockRepoMetadataOptions{reviewers: false, teamReviewers: false, assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateApiActors(reg)
				mockRequestReviewsByLogin(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive skip reviewers",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Title.Edited = true
						e.Body.Edited = true
						e.Assignees.Edited = true
						e.Labels.Edited = true
						e.Projects.Edited = true
						e.Milestone.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						e.Title.Value = "new title"
						e.Body.Value = "new body"
						// When ApiActorsSupported is enabled, the interactive flow returns display names (or logins for non-users)
						e.Assignees.Value = []string{"monalisa (Mona Display Name)", "hubot"}
						// Populate metadata to simulate what searchFunc would do during prompting
						e.Metadata.AssignableActors = []api.AssignableActor{
							api.NewAssignableBot("HUBOTID", "hubot"),
							api.NewAssignableUser("MONAID", "monalisa", "Mona Display Name"),
						}
						e.Labels.Value = []string{"feature", "TODO", "bug"}
						e.Labels.Add = []string{"feature", "TODO", "bug"}
						e.Labels.Remove = []string{"docs"}
						e.Projects.Value = []string{"Cleanup", "CleanupV2"}
						e.Milestone.Value = "GA"
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// interactive but reviewers not chosen; need everything except reviewers/teams
				// assignees: false because searchFunc handles dynamic fetching (metadata populated in test mock)
				mockRepoMetadata(reg, mockRepoMetadataOptions{assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockPullRequestUpdateApiActors(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive remove all reviewers",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{ // include existing reviewers
					URL: "https://github.com/OWNER/REPO/pull/123",
					ReviewRequests: api.ReviewRequests{Nodes: []struct{ RequestedReviewer api.RequestedReviewer }{
						{RequestedReviewer: api.RequestedReviewer{TypeName: "Team", Slug: "core", Organization: struct {
							Login string `json:"login"`
						}{Login: "OWNER"}}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "Team", Slug: "external", Organization: struct {
							Login string `json:"login"`
						}{Login: "OWNER"}}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "monalisa"}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "hubot"}},
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "dependabot"}},
					}},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Title.Edited = true
						e.Body.Edited = true
						e.Reviewers.Edited = true
						e.Assignees.Edited = true
						e.Labels.Edited = true
						e.Projects.Edited = true
						e.Milestone.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						e.Title.Value = "new title"
						e.Body.Value = "new body"
						e.Reviewers.Remove = []string{"monalisa", "hubot", "OWNER/core", "OWNER/external", "dependabot"}
						e.Assignees.Value = []string{"monalisa", "hubot"}
						// Populate metadata to simulate what searchFunc would do during prompting
						e.Metadata.AssignableActors = []api.AssignableActor{
							api.NewAssignableBot("HUBOTID", "hubot"),
							api.NewAssignableUser("MONAID", "monalisa", "Mona Display Name"),
						}
						// Populate team metadata for reviewer search
						e.Metadata.Teams = []api.OrgTeam{
							{ID: "COREID", Slug: "core"},
							{ID: "EXTERNALID", Slug: "external"},
						}
						e.Labels.Value = []string{"feature", "TODO", "bug"}
						e.Labels.Add = []string{"feature", "TODO", "bug"}
						e.Labels.Remove = []string{"docs"}
						e.Projects.Value = []string{"Cleanup", "CleanupV2"}
						e.Milestone.Value = "GA"
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// With search functions enabled, we don't fetch reviewers/assignees metadata
				mockRepoMetadata(reg, mockRepoMetadataOptions{reviewers: false, teamReviewers: false, assignees: false, labels: true, projects: true, milestones: true})
				mockPullRequestUpdate(reg)
				mockRequestReviewsByLogin(reg)
				mockPullRequestUpdateApiActors(reg)
				mockPullRequestUpdateLabels(reg)
				mockProjectV2ItemUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive prompts with actor assignee display names when actors available",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
					AssignedActors: api.AssignedActors{
						Nodes: []api.Actor{
							{
								ID:       "HUBOTID",
								Login:    "hubot",
								TypeName: "Bot",
							},
						},
						TotalCount: 1,
					},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Assignees.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						// Checking that the display name is being used in the prompt.
						require.Equal(t, []string{"hubot"}, e.Assignees.Default)
						require.Equal(t, []string{"hubot"}, e.Assignees.DefaultLogins)

						// Adding monalisa as PR assignee, should preserve hubot.
						// MultiSelectWithSearch returns Keys (logins), not display names.
						e.Assignees.Value = []string{"hubot", "monalisa"}
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdate(reg)
				reg.Register(
					httpmock.GraphQL(`mutation ReplaceActorsForAssignable\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "replaceActorsForAssignable": { "__typename": "" } } }`,
						func(inputs map[string]any) {
							require.Subset(t, inputs["actorLogins"], []any{"hubot", "monalisa"})
						}),
				)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "Legacy assignee users are fetched and updated on unsupported GitHub Hosts",
			input: &EditOptions{
				Detector:    &fd.DisabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Assignees: shared.EditableAssignees{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"monalisa", "hubot"},
							Remove: []string{"octocat"},
							Edited: true,
						},
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Notice there is no call to mockReplaceActorsForAssignable()
				// and no GraphQL call to RepositoryAssignableActors below.
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "assignableUsers": {
						"nodes": [
							{ "login": "hubot", "id": "HUBOTID" },
							{ "login": "monalisa", "id": "MONAID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				mockPullRequestUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive GHES uses legacy assignee flow without search",
			input: &EditOptions{
				Detector:    &fd.DisabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
					Assignees: api.Assignees{
						Nodes:      []api.GitHubUser{{Login: "octocat", ID: "OCTOID"}},
						TotalCount: 1,
					},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Assignees.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						require.False(t, e.ApiActorsSupported)
						require.Nil(t, e.AssigneeSearchFunc)
						require.Contains(t, e.Assignees.Options, "monalisa")
						require.Contains(t, e.Assignees.Options, "hubot")

						e.Assignees.Value = []string{"monalisa", "hubot"}
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.GraphQL(`query RepositoryAssignableActors\b`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "assignableUsers": {
						"nodes": [
							{ "login": "hubot", "id": "HUBOTID" },
							{ "login": "monalisa", "id": "MONAID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				reg.Exclude(t, httpmock.GraphQL(`mutation ReplaceActorsForAssignable\b`))
				mockPullRequestUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "interactive GHES uses legacy reviewer flow without search",
			input: &EditOptions{
				Detector:    &fd.DisabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
					ReviewRequests: api.ReviewRequests{Nodes: []struct{ RequestedReviewer api.RequestedReviewer }{
						{RequestedReviewer: api.RequestedReviewer{TypeName: "User", Login: "octocat"}},
					}},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Reviewers.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, _ string) error {
						// Verify GHES uses legacy flow: ReviewerSearchFunc should be nil
						require.Nil(t, e.ReviewerSearchFunc)
						// Verify options are populated from fetched metadata
						require.Contains(t, e.Reviewers.Options, "monalisa")
						require.Contains(t, e.Reviewers.Options, "hubot")
						require.Contains(t, e.Reviewers.Options, "OWNER/core")

						e.Reviewers.Value = []string{"monalisa", "OWNER/core"}
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// GHES should NOT use the new SuggestedReviewerActors query
				reg.Exclude(t, httpmock.GraphQL(`query SuggestedReviewerActors\b`))
				// GHES should use legacy metadata fetch for reviewers (AssignableUsers, not Actors)
				reg.Exclude(t, httpmock.GraphQL(`query RepositoryAssignableActors\b`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "assignableUsers": {
						"nodes": [
							{ "login": "hubot", "id": "HUBOTID" },
							{ "login": "monalisa", "id": "MONAID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				// GHES should fetch teams for interactive reviewer editing
				reg.Register(
					httpmock.GraphQL(`query OrganizationTeamList\b`),
					httpmock.StringResponse(`
					{ "data": { "organization": { "teams": {
						"nodes": [
							{ "slug": "external", "id": "EXTERNALID" },
							{ "slug": "core", "id": "COREID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				// Current user fetched for reviewers
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`
					{ "data": { "viewer": { "login": "monalisa" } } }
					`))
				mockPullRequestUpdate(reg)
				mockPullRequestAddReviewers(reg)
				mockPullRequestRemoveReviewers(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "non-interactive projects v1 unsupported doesn't fetch v1 metadata",
			input: &EditOptions{
				Detector:    &fd.DisabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL: "https://github.com/OWNER/REPO/pull/123",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Projects: shared.EditableProjects{
						EditableSlice: shared.EditableSlice{
							Add:    []string{"CleanupV2"},
							Remove: []string{"RoadmapV2"},
							Edited: true,
						},
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Ensure v1 project queries are NOT made.
				reg.Exclude(t, httpmock.GraphQL(`query RepositoryProjectList\b`))
				reg.Exclude(t, httpmock.GraphQL(`query OrganizationProjectList\b`))
				// Provide only v2 project metadata queries.
				reg.Register(
					httpmock.GraphQL(`query RepositoryProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "projectsV2": {
						"nodes": [
							{ "title": "CleanupV2", "id": "CLEANUPV2ID" },
							{ "title": "RoadmapV2", "id": "ROADMAPV2ID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "organization": { "projectsV2": {
						"nodes": [
							{ "title": "TriageV2", "id": "TRIAGEV2ID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query UserProjectV2List\b`),
					httpmock.StringResponse(`
					{ "data": { "viewer": { "projectsV2": {
						"nodes": [
							{ "title": "MonalisaV2", "id": "MONALISAV2ID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				mockProjectV2ItemUpdate(reg)
				mockPullRequestUpdate(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "cancelling the editor uploads nothing",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Body.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, editorCmd string) error {
						return errors.New("the user quit the editor")
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			attach: []string{"shot.png"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.REST("POST", "user-attachments/assets"))
			},
			wantErr: "the user quit the editor",
		},
		{
			name: "a token that cannot upload fails before any prompt",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive:     true,
				Surveyor:        testSurveyor{},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			attach:  []string{"shot.png"},
			token:   "ghs_anactionstoken",
			wantErr: "unsupported authentication type",
		},
		{
			name: "attaching with no body flag keeps the body already on the pull request",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithBody(t, reg, "the original body\n\n![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "an empty body flag clears the body and leaves the attachment",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Body: shared.EditableString{
						Value:  "",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithBody(t, reg, "![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "attaching does not double what the editor already carried",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error {
						e.Body.Edited = true
						return nil
					},
					editFields: func(e *shared.Editable, editorCmd string) error {
						e.Body.Value = e.Body.Default + "\n\nplus an edit"
						return nil
					},
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithBody(t, reg, "the original body\n\nplus an edit\n\n![shot](https://example.com/1)")
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "an upload that partly succeeded writes the part that uploaded",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach: []string{"a.png", "b.png"},
			uploads: []attachments.UploadStub{
				{Name: "a.png", Status: 201, Body: `{"url":"https://example.com/1"}`},
				{Name: "b.png", Status: 404, Body: `{"message":"Not Found"}`},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithBody(t, reg, "the original body\n\n![a](https://example.com/1)")
			},
			stdout:  "https://github.com/OWNER/REPO/pull/123\n",
			wantErr: "could not upload ./b.png: attaching files requires write access to the repository",
		},
		{
			name: "a sole failed upload leaves the body alone and still edits the title",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{Value: "new title", Edited: true},
				},
				Fetcher: testFetcher{},
			},
			attach:  []string{"a.png"},
			uploads: []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithoutBody(t, reg, "new title")
			},
			stdout:  "https://github.com/OWNER/REPO/pull/123\n",
			wantErr: "could not upload ./a.png: attaching files requires write access to the repository",
		},
		{
			name: "a validation failure leaves the body alone and still edits the title",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "important work\n\n![demo][d]\n\n[d]: ./demo.mp4",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{Value: "new title", Edited: true},
				},
				Fetcher: testFetcher{},
			},
			attach: []string{"demo.mp4"},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.REST("POST", "user-attachments/assets"))
				mockPullRequestUpdateWithoutBody(t, reg, "new title")
			},
			stdout:  "https://github.com/OWNER/REPO/pull/123\n",
			wantErr: "cannot embed a video as a reference-style image: ./demo.mp4",
		},
		{
			name: "an attach that writes nothing does not print the URL",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:  []string{"a.png"},
			uploads: []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.GraphQL(`mutation PullRequestUpdate\b`))
			},
			wantErr: "could not upload ./a.png: attaching files requires write access to the repository\nthe pull request was not changed",
		},
		{
			name: "an upload failure and a write failure are both reported",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{Value: "new title", Edited: true},
				},
				Fetcher: testFetcher{},
			},
			attach:  []string{"a.png"},
			uploads: []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestUpdate\b`),
					httpmock.StatusStringResponse(500, `{"message":"the write failed"}`),
				)
			},
			wantErr: "could not upload ./a.png: attaching files requires write access to the repository\nnon-200 OK status code:  body: \"{\\\"message\\\":\\\"the write failed\\\"}\"",
		},
		{
			// This predates --attach, which is why the not-changed message the
			// row above covers is guarded on something being attached.
			name: "an interactive run that selects nothing still prints the URL",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:  "https://github.com/OWNER/REPO/pull/123",
					Body: "the original body",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: true,
				Surveyor: testSurveyor{
					fieldsToEdit: func(e *shared.Editable) error { return nil },
					editFields:   func(e *shared.Editable, editorCmd string) error { return nil },
				},
				Fetcher:         testFetcher{},
				EditorRetriever: testEditorRetriever{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.GraphQL(`mutation PullRequestUpdate\b`))
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "a failed upload drops even a body the caller typed",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{Value: "new title", Edited: true},
					Body:  shared.EditableString{Value: "see ![a](./a.png)", Edited: true},
				},
				Fetcher: testFetcher{},
			},
			attach:  []string{"a.png"},
			uploads: []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithoutBody(t, reg, "new title")
			},
			stdout:  "https://github.com/OWNER/REPO/pull/123\n",
			wantErr: "could not upload ./a.png: attaching files requires write access to the repository",
		},
		{
			// The default host deliberately holds a token that cannot upload,
			// so tidying these fixture tokens weakens the row without failing it.
			name: "the token comes from the host the pull request was fetched from",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "https://acme.ghe.com/OWNER/REPO/pull/123",
				Finder: shared.NewMockFinder("https://acme.ghe.com/OWNER/REPO/pull/123", &api.PullRequest{
					URL:        "https://acme.ghe.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.NewWithHost("OWNER", "REPO", "acme.ghe.com")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:         []string{"shot.png"},
			hostTokens:     map[string]string{"github.com": "ghs_anactionstoken", "acme.ghe.com": "gho_atenanttoken"},
			uploads:        []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			wantUploadHost: "uploads.acme.ghe.com",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdateWithBody(t, reg, "the original body\n\n![shot](https://example.com/1)")
			},
			stdout: "https://acme.ghe.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "a missing repository id stops the upload",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Repository: &api.PRRepository{DatabaseID: 0, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:  []string{"shot.png"},
			wantErr: "could not determine which repository to attach files to",
		},
		{
			name: "read access stops the upload",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "READ"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:  []string{"shot.png"},
			wantErr: "attaching files requires write access to the repository",
		},
		{
			name: "attaching asks the pull request lookup for the repository id",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:        "https://github.com/OWNER/REPO/pull/123",
					Body:       "the original body",
					Repository: &api.PRRepository{DatabaseID: 1234, ViewerPermission: "WRITE"},
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Fetcher:     testFetcher{},
			},
			attach:  []string{"shot.png"},
			uploads: []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdate(reg)
			},
			wantLookupFields: []string{"repository"},
			stdout:           "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "a run that attaches nothing does not write the body",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:  "https://github.com/OWNER/REPO/pull/123",
					Body: "the original body",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Labels: shared.EditableSlice{
						Add:    []string{"bug"},
						Remove: []string{"docs"},
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Exclude(t, httpmock.GraphQL(`mutation PullRequestUpdate\b`))
				mockRepoMetadata(reg, mockRepoMetadataOptions{labels: true})
				mockPullRequestUpdateLabels(reg)
			},
			stdout: "https://github.com/OWNER/REPO/pull/123\n",
		},
		{
			name: "title-only lookup avoids unrelated fields",
			input: &EditOptions{
				Detector:    &fd.EnabledDetectorMock{},
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					URL:  "https://github.com/OWNER/REPO/pull/123",
					Body: "the original body",
				}, ghrepo.New("OWNER", "REPO")),
				Interactive: false,
				Editable: shared.Editable{
					Title: shared.EditableString{
						Value:  "new title",
						Edited: true,
					},
				},
				Fetcher: testFetcher{},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockPullRequestUpdate(reg)
			},
			wantLookupFields:   []string{"id", "url"},
			wantNoLookupFields: []string{"author", "title", "body", "baseRefName", "reviewRequests", "assignedActors", "assignees", "labels", "projectCards", "projectItems", "milestone", "repository"},
			stdout:             "https://github.com/OWNER/REPO/pull/123\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			for _, u := range tt.uploads {
				if tt.wantUploadHost != "" {
					attachments.StubUploadToHost(t, reg, tt.wantUploadHost, 1234, u.Name, u.Status, u.Body)
					continue
				}
				attachments.StubUpload(reg, 1234, u.Name, u.Status, u.Body)
			}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			httpClient := func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
			baseRepo := func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil }

			tt.input.IO = ios
			tt.input.HttpClient = httpClient
			tt.input.BaseRepo = baseRepo

			// NewTestAssets moves into a temporary directory, so it runs before
			// anything else reads a relative path.
			if len(tt.attach) > 0 {
				tt.input.Assets = attachments.NewTestAssets(t, tt.attach...)
			}

			// The host comes from the pull request the row's finder returns, so
			// a row can hold a token for a host the config's default is not.
			hostTokens := tt.hostTokens
			if hostTokens == nil {
				token := tt.token
				if token == "" {
					token = "gho_atokenthatcanupload"
				}
				hostTokens = map[string]string{"github.com": token}
			}
			tt.input.Config = func() (gh.Config, error) {
				return config.NewMockConfigFromString(hostsConfig(hostTokens)), nil
			}

			var lookupFields []string
			tt.input.Finder = fieldCapturingFinder{PRFinder: tt.input.Finder, fields: &lookupFields}

			err := editRun(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())

			for _, field := range tt.wantLookupFields {
				assert.Contains(t, lookupFields, field)
			}
			for _, field := range tt.wantNoLookupFields {
				assert.NotContains(t, lookupFields, field)
			}
		})
	}
}

func Test_pullRequestLookupFields(t *testing.T) {
	tests := []struct {
		name                string
		editable            shared.Editable
		interactive         bool
		hasAssets           bool
		apiActorsSupported  bool
		projectsV1Supported bool
		want                []string
	}{
		{
			name: "title",
			editable: shared.Editable{
				Title: shared.EditableString{Edited: true},
			},
			want: []string{"id", "url"},
		},
		{
			name: "body",
			editable: shared.Editable{
				Body: shared.EditableString{Edited: true},
			},
			want: []string{"id", "url"},
		},
		{
			name: "base",
			editable: shared.Editable{
				Base: shared.EditableString{Edited: true},
			},
			want: []string{"id", "url"},
		},
		{
			name: "reviewers",
			editable: shared.Editable{
				Reviewers: shared.EditableReviewers{
					EditableSlice: shared.EditableSlice{Edited: true},
				},
			},
			want: []string{"id", "url", "reviewRequests"},
		},
		{
			name: "assignees with actors",
			editable: shared.Editable{
				Assignees: shared.EditableAssignees{
					EditableSlice: shared.EditableSlice{Edited: true},
				},
			},
			apiActorsSupported: true,
			want:               []string{"id", "url", "assignedActors"},
		},
		{
			name: "assignees without actors",
			editable: shared.Editable{
				Assignees: shared.EditableAssignees{
					EditableSlice: shared.EditableSlice{Edited: true},
				},
			},
			want: []string{"id", "url", "assignees"},
		},
		{
			name: "labels",
			editable: shared.Editable{
				Labels: shared.EditableSlice{Edited: true},
			},
			want: []string{"id", "url"},
		},
		{
			name: "projects v1 add",
			editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Add: []string{"Roadmap"}, Edited: true},
				},
			},
			projectsV1Supported: true,
			want:                []string{"id", "url", "projectCards"},
		},
		{
			name: "projects v2 add",
			editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Add: []string{"Roadmap"}, Edited: true},
				},
			},
			want: []string{"id", "url"},
		},
		{
			name: "projects v2 remove",
			editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Remove: []string{"Roadmap"}, Edited: true},
				},
			},
			want: []string{"id", "url", "projectItems"},
		},
		{
			name: "projects v1 remove",
			editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Remove: []string{"Roadmap"}, Edited: true},
				},
			},
			projectsV1Supported: true,
			want:                []string{"id", "url", "projectCards", "projectItems"},
		},
		{
			name: "milestone",
			editable: shared.Editable{
				Milestone: shared.EditableString{Edited: true},
			},
			want: []string{"id", "url"},
		},
		{
			name:      "attachment",
			hasAssets: true,
			want:      []string{"id", "url", "body", "repository"},
		},
		{
			name: "attachment with replacement body",
			editable: shared.Editable{
				Body: shared.EditableString{Edited: true},
			},
			hasAssets: true,
			want:      []string{"id", "url", "repository"},
		},
		{
			name:                "interactive with actors",
			interactive:         true,
			apiActorsSupported:  true,
			projectsV1Supported: true,
			want:                []string{"id", "url", "author", "title", "body", "baseRefName", "reviewRequests", "labels", "projectCards", "projectItems", "milestone", "assignedActors"},
		},
		{
			name:        "interactive without actors and with attachment",
			interactive: true,
			hasAssets:   true,
			want:        []string{"id", "url", "author", "title", "body", "baseRefName", "reviewRequests", "labels", "projectItems", "milestone", "assignees", "repository"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pullRequestLookupFields(tt.editable, tt.interactive, tt.hasAssets, tt.apiActorsSupported, tt.projectsV1Supported)
			assert.Equal(t, tt.want, got)
		})
	}
}

// fieldCapturingFinder records the fields a run asks the pull request lookup
// for, then delegates to the finder it wraps.
type fieldCapturingFinder struct {
	shared.PRFinder
	fields *[]string
}

func (f fieldCapturingFinder) Find(opts shared.FindOptions) (*api.PullRequest, ghrepo.Interface, error) {
	*f.fields = opts.Fields
	return f.PRFinder.Find(opts)
}

func hostsConfig(tokens map[string]string) string {
	var b strings.Builder
	b.WriteString("hosts:\n")
	for host, token := range tokens {
		fmt.Fprintf(&b, "  %s:\n    user: monalisa\n    oauth_token: %s\n", host, token)
	}
	return b.String()
}

type mockRepoMetadataOptions struct {
	reviewers     bool
	teamReviewers bool // reviewers must also be true for this to have an effect.
	assignees     bool
	labels        bool
	projects      bool // includes both legacy (v1) and v2
	milestones    bool
}

func mockRepoMetadata(reg *httpmock.Registry, opt mockRepoMetadataOptions) {
	// Assignable actors (users/bots) are fetched when reviewers OR assignees edited with ApiActorsSupported enabled.
	if opt.reviewers || opt.assignees {
		reg.Register(
			httpmock.GraphQL(`query RepositoryAssignableActors\b`),
			httpmock.StringResponse(`
			{ "data": { "repository": { "suggestedActors": {
				"nodes": [
					{ "login": "hubot", "id": "HUBOTID", "__typename": "Bot" },
					{ "login": "monalisa", "id": "MONAID", "name": "Mona Display Name", "__typename": "User" }
				],
				"pageInfo": { "hasNextPage": false }
			} } } }
			`))
	}
	if opt.labels {
		reg.Register(
			httpmock.GraphQL(`query RepositoryLabelList\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "labels": {
			"nodes": [
				{ "name": "feature", "id": "FEATUREID" },
				{ "name": "TODO", "id": "TODOID" },
				{ "name": "bug", "id": "BUGID" },
				{ "name": "docs", "id": "DOCSID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	}
	if opt.milestones {
		reg.Register(
			httpmock.GraphQL(`query RepositoryMilestoneList\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	}
	if opt.projects {
		reg.Register(
			httpmock.GraphQL(`query RepositoryProjectList\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		reg.Register(
			httpmock.GraphQL(`query OrganizationProjectList\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projects": {
			"nodes": [
				{ "name": "Triage", "id": "TRIAGEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		reg.Register(
			httpmock.GraphQL(`query RepositoryProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "repository": { "projectsV2": {
			"nodes": [
				{ "title": "CleanupV2", "id": "CLEANUPV2ID" },
				{ "title": "RoadmapV2", "id": "ROADMAPV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		reg.Register(
			httpmock.GraphQL(`query OrganizationProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "organization": { "projectsV2": {
			"nodes": [
				{ "title": "TriageV2", "id": "TRIAGEV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
		reg.Register(
			httpmock.GraphQL(`query UserProjectV2List\b`),
			httpmock.StringResponse(`
		{ "data": { "viewer": { "projectsV2": {
			"nodes": [
				{ "title": "MonalisaV2", "id": "MONALISAV2ID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	}
	if opt.teamReviewers && opt.reviewers { // teams only relevant if reviewers edited
		reg.Register(
			httpmock.GraphQL(`query OrganizationTeamList\b`),
			httpmock.StringResponse(`
      { "data": { "organization": { "teams": {
        "nodes": [
          { "slug": "external", "id": "EXTERNALID" },
          { "slug": "core", "id": "COREID" }
        ],
        "pageInfo": { "hasNextPage": false }
      } } } }
		`))
	}
	if opt.reviewers { // Current user fetched only when reviewers requested
		reg.Register(
			httpmock.GraphQL(`query UserCurrent\b`),
			httpmock.StringResponse(`
	  { "data": { "viewer": { "login": "monalisa" } } }
	`))
	}
}

func mockPullRequestUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdate\b`),
		httpmock.StringResponse(`{}`))
}

func mockPullRequestUpdateWithBody(t *testing.T, reg *httpmock.Registry, wantBody string) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdate\b`),
		httpmock.GraphQLMutation(`{}`, func(inputs map[string]any) {
			assert.Equal(t, wantBody, inputs["body"])
		}),
	)
}

// A dropped body is absent from the input, which is not the same as a body sent
// with its old value.
func mockPullRequestUpdateWithoutBody(t *testing.T, reg *httpmock.Registry, wantTitle string) {
	reg.Register(
		httpmock.GraphQL(`mutation PullRequestUpdate\b`),
		httpmock.GraphQLMutation(`{}`, func(inputs map[string]any) {
			assert.Equal(t, wantTitle, inputs["title"])
			assert.NotContains(t, inputs, "body")
		}),
	)
}

func mockPullRequestUpdateApiActors(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation ReplaceActorsForAssignable\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "replaceActorsForAssignable": { "__typename": "" } } }`,
			func(inputs map[string]any) {}),
	)
}

func mockPullRequestAddReviewers(reg *httpmock.Registry) {
	reg.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/pulls/0/requested_reviewers"),
		httpmock.StringResponse(`{}`))
}

func mockPullRequestRemoveReviewers(reg *httpmock.Registry) {
	reg.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/pulls/0/requested_reviewers"),
		httpmock.StringResponse(`{}`))
}

// mockRequestReviewsByLogin mocks the RequestReviewsByLogin GraphQL mutation
// used on github.com when ApiActorsSupported is enabled.
func mockRequestReviewsByLogin(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation RequestReviewsByLogin\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "requestReviewsByLogin": { "clientMutationId": "" } } }`,
			func(inputs map[string]any) {}),
	)
}

func mockPullRequestUpdateLabels(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation LabelAdd\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "addLabelsToLabelable": { "__typename": "" } } }`,
			func(inputs map[string]any) {}),
	)
	reg.Register(
		httpmock.GraphQL(`mutation LabelRemove\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "removeLabelsFromLabelable": { "__typename": "" } } }`,
			func(inputs map[string]any) {}),
	)
}

func mockProjectV2ItemUpdate(reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "add_000": { "item": { "id": "1" } }, "delete_001": { "item": { "id": "2" } } } }`,
			func(inputs map[string]any) {}),
	)
}

type testFetcher struct{}

func (f testFetcher) EditableOptionsFetch(client *api.Client, repo ghrepo.Interface, opts *shared.Editable, projectsV1Support gh.ProjectsV1Support) error {
	return shared.FetchOptions(client, repo, opts, projectsV1Support)
}

type testSurveyor struct {
	fieldsToEdit func(e *shared.Editable) error
	editFields   func(e *shared.Editable, editorCmd string) error
}

func (s testSurveyor) FieldsToEdit(e *shared.Editable) error {
	return s.fieldsToEdit(e)
}

func (s testSurveyor) EditFields(e *shared.Editable, editorCmd string) error {
	return s.editFields(e, editorCmd)
}

type testEditorRetriever struct{}

func (t testEditorRetriever) Retrieve() (string, error) {
	return "vim", nil
}

// TODO projectsV1Deprecation
// Remove this test.
func TestProjectsV1Deprecation(t *testing.T) {
	t.Run("when projects v1 is supported, is included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`projectCards`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		f := &cmdutil.Factory{
			IOStreams: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
		}

		// Ignore the error because we have no way to really stub it without
		// fully stubbing a GQL error structure in the request body.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			Detector: &fd.EnabledDetectorMock{},

			Finder: shared.NewFinder(f),

			SelectorArg: "https://github.com/cli/cli/pull/123",
			Editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Edited: true},
				},
			},
		})

		// Verify that our request contained projectCards
		reg.Verify(t)
	})

	t.Run("when projects v1 is not supported, is not included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Exclude(t, httpmock.GraphQL(`projectCards`))

		f := &cmdutil.Factory{
			IOStreams: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
		}

		// Ignore the error because we have no way to really stub it without
		// fully stubbing a GQL error structure in the request body.
		_ = editRun(&EditOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			Detector: &fd.DisabledDetectorMock{},

			Finder: shared.NewFinder(f),

			SelectorArg: "https://github.com/cli/cli/pull/123",
			Editable: shared.Editable{
				Projects: shared.EditableProjects{
					EditableSlice: shared.EditableSlice{Edited: true},
				},
			},
		})

		// Verify that our request did not contain projectCards
		reg.Verify(t)
	})
}
