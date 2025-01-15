package link

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdLink(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         linkOpts
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
			name:        "specify-repo-and-team",
			cli:         "--repo my-repo --team my-team",
			wantsErr:    true,
			wantsErrMsg: "specify only one of `--repo` or `--team`",
		},
		{
			name: "specify-nothing",
			cli:  "",
			wants: linkOpts{
				owner: "OWNER",
				repo:  "REPO",
			},
		},
		{
			name: "repo",
			cli:  "--repo my-repo",
			wants: linkOpts{
				repo: "my-repo",
			},
		},
		{
			name: "repo-flag-contains-owner",
			cli:  "--repo monalisa/my-repo",
			wants: linkOpts{
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name: "repo-flag-contains-owner-and-host",
			cli:  "--repo github.com/monalisa/my-repo",
			wants: linkOpts{
				host:  "github.com",
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name:        "repo-flag-contains-wrong-format",
			cli:         "--repo h/e/l/l/o",
			wantsErr:    true,
			wantsErrMsg: "expected the \"[HOST/]OWNER/REPO\" or \"REPO\" format, got \"h/e/l/l/o\"",
		},
		{
			name:        "repo-flag-with-owner-different-from-owner-flag",
			cli:         "--repo monalisa/my-repo --owner leonardo",
			wantsErr:    true,
			wantsErrMsg: "'monalisa/my-repo' has different owner from 'leonardo'",
		},
		{
			name: "team",
			cli:  "--team my-team",
			wants: linkOpts{
				team: "my-team",
			},
		},
		{
			name: "team-flag-contains-owner",
			cli:  "--team my-org/my-team",
			wants: linkOpts{
				owner: "my-org",
				team:  "my-team",
			},
		},
		{
			name: "team-flag-contains-owner-and-host",
			cli:  "--team github.com/my-org/my-team",
			wants: linkOpts{
				host:  "github.com",
				owner: "my-org",
				team:  "my-team",
			},
		},
		{
			name:        "team-flag-contains-wrong-format",
			cli:         "--team h/e/l/l/o",
			wantsErr:    true,
			wantsErrMsg: "expected the \"[HOST/]OWNER/TEAM\" or \"TEAM\" format, got \"h/e/l/l/o\"",
		},
		{
			name:        "team-flag-with-owner-different-from-owner-flag",
			cli:         "--team my-org/my-team --owner her-org",
			wantsErr:    true,
			wantsErrMsg: "'my-org/my-team' has different owner from 'her-org'",
		},
		{
			name: "number",
			cli:  "123 --repo my-repo",
			wants: linkOpts{
				number: 123,
				repo:   "my-repo",
			},
		},
		{
			name: "owner-with-repo-flag",
			cli:  "--repo my-repo --owner monalisa",
			wants: linkOpts{
				owner: "monalisa",
				repo:  "my-repo",
			},
		},
		{
			name: "owner-without-repo-flag",
			cli:  "--owner monalisa",
			wants: linkOpts{
				owner: "monalisa",
				repo:  "REPO",
			},
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			}

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)

			var gotOpts linkOpts
			cmd := NewCmdLink(f, func(config linkConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				require.Error(t, err)
				require.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.wants.number, gotOpts.number)
			require.Equal(t, tt.wants.owner, gotOpts.owner)
			require.Equal(t, tt.wants.repo, gotOpts.repo)
			require.Equal(t, tt.wants.team, gotOpts.team)
			require.Equal(t, tt.wants.projectID, gotOpts.projectID)
			require.Equal(t, tt.wants.repoID, gotOpts.repoID)
			require.Equal(t, tt.wants.teamID, gotOpts.teamID)
		})
	}
}

func TestRunLink_Repo(t *testing.T) {
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
						"id":    "project-ID",
						"title": "first-project",
					},
				},
			},
		})

	// link projectV2 to repository
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "mutation LinkProjectV2ToRepository.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"linkProjectV2ToRepository": map[string]interface{}{},
			},
		})

	// get repo ID
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`.*query RepositoryInfo.*`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"repository": map[string]interface{}{
					"id": "repo-ID",
				},
			},
		})

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	cfg := linkConfig{
		opts: linkOpts{
			number: 1,
			repo:   "my-repo",
			owner:  "monalisa",
		},
		client: queries.NewTestClient(),
		httpClient: func() (*http.Client, error) {
			return http.DefaultClient, nil
		},
		config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		io: ios,
	}

	err := runLink(cfg)
	require.NoError(t, err)
	require.Equal(
		t,
		"Linked 'monalisa/my-repo' to project #1 'first-project'\n",
		stdout.String())
}

func TestRunLink_Team(t *testing.T) {
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
						"id":    "project-ID",
						"title": "first-project",
					},
				},
			},
		})

	// link projectV2 to team
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "mutation LinkProjectV2ToTeam.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"linkProjectV2ToTeam": map[string]interface{}{},
			},
		})

	// get team ID
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`.*query OrganizationTeam.*`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"team": map[string]interface{}{
						"id": "team-ID",
					},
				},
			},
		})

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	cfg := linkConfig{
		opts: linkOpts{
			number: 1,
			team:   "my-team",
			owner:  "monalisa-org",
		},
		client: queries.NewTestClient(),
		httpClient: func() (*http.Client, error) {
			return http.DefaultClient, nil
		},
		config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		io: ios,
	}

	err := runLink(cfg)
	require.NoError(t, err)
	require.Equal(
		t,
		"Linked 'monalisa-org/my-team' to project #1 'first-project'\n",
		stdout.String())
}
