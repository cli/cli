package unlink

import (
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
	"net/http"
	"testing"
)

func TestNewCmdUnlink(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         unlinkOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "not-a-number",
			cli:         "x",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name:        "specify-nothing",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "specify either `--repo` or `--team`",
		},
		{
			name:        "specify-repo-and-team",
			cli:         "--repo my-repo --team my-team",
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--repo` or `--team`",
		},
		{
			name: "repo",
			cli:  "--repo my-repo",
			wants: unlinkOpts{
				repo: "my-repo",
			},
		},
		{
			name: "team",
			cli:  "--team my-team",
			wants: unlinkOpts{
				team: "my-team",
			},
		},
		{
			name: "number",
			cli:  "123 --repo my-repo",
			wants: unlinkOpts{
				number: 123,
				repo:   "my-repo",
			},
		},
		{
			name: "owner",
			cli:  "--repo my-repo --owner monalisa",
			wants: unlinkOpts{
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name: "json",
			cli:  "--repo my-repo --format json",
			wants: unlinkOpts{
				repo: "my-repo",
			},
			wantsExporter: true,
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts unlinkOpts
			cmd := NewCmdUnlink(f, func(config unlinkConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.repo, gotOpts.repo)
			assert.Equal(t, tt.wants.team, gotOpts.team)
			assert.Equal(t, tt.wants.projectID, gotOpts.projectID)
			assert.Equal(t, tt.wants.repoID, gotOpts.repoID)
			assert.Equal(t, tt.wants.teamID, gotOpts.teamID)
			assert.Equal(t, tt.wants.format, gotOpts.format)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func newTestClient(reg *httpmock.Registry) *http.Client {
	client := &http.Client{}
	httpmock.ReplaceTripper(client, reg)
	return client
}

func TestRunUnlink_Repo(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]string{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]string{
						"id": "project-ID",
					},
				},
			},
		})

	// unlink projectV2 from repository
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "mutation UnlinkProjectV2FromRepository.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"unlinkProjectV2FromRepository": map[string]interface{}{
					"repository": map[string]interface{}{
						"name": "my-repo",
						"id":   "repo-ID",
						"url":  "http://a-url.com",
					},
				},
			},
		})
	client := queries.NewTestClient()

	// get repo ID
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`{"data":{"repository":{"id": "repo-ID"}}}`),
	)
	httpClient := newTestClient(httpReg)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := unlinkConfig{
		opts: unlinkOpts{
			number: 1,
			repo:   "my-repo",
			owner:  "monalisa",
		},
		client: client,
		httpClient: func() (*http.Client, error) {
			return httpClient, nil
		},
		io: ios,
	}

	err := runUnlink(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}

func TestRunUnlink_Team(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]string{
				"login": "monalisa-org",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa-org",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// get user project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa-org",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]string{
						"id": "project-ID",
					},
				},
			},
		})

	// unlink projectV2 from team
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "mutation UnlinkProjectV2FromTeam.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"unlinkProjectV2FromTeam": map[string]interface{}{
					"team": map[string]interface{}{
						"id":  "team-ID",
						"url": "http://a-url.com",
					},
				},
			},
		})
	client := queries.NewTestClient()

	// get team ID
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query OrganizationTeam\b`),
		httpmock.StringResponse(`{"data":{"organization":{"team":{"id": "team-ID"}}}}`),
	)
	httpClient := newTestClient(httpReg)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := unlinkConfig{
		opts: unlinkOpts{
			number: 1,
			team:   "my-team",
			owner:  "monalisa-org",
		},
		client: client,
		httpClient: func() (*http.Client, error) {
			return httpClient, nil
		},
		io: ios,
	}

	err := runUnlink(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"http://a-url.com\n",
		stdout.String())
}
