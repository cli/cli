package list

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants ListOptions
	}{
		{
			name: "repo",
			cli:  "",
			wants: ListOptions{
				OrgName: "",
			},
		},
		{
			name: "org",
			cli:  "-oUmbrellaCorporation",
			wants: ListOptions{
				OrgName: "UmbrellaCorporation",
			},
		},
		{
			name: "env",
			cli:  "-eDevelopment",
			wants: ListOptions{
				EnvName: "Development",
			},
		},
		{
			name: "user",
			cli:  "-u",
			wants: ListOptions{
				UserSecrets: true,
			},
		},
		{
			name: "Dependabot repo",
			cli:  "--app Dependabot",
			wants: ListOptions{
				Application: "Dependabot",
			},
		},
		{
			name: "Dependabot org",
			cli:  "--app Dependabot --org UmbrellaCorporation",
			wants: ListOptions{
				Application: "Dependabot",
				OrgName:     "UmbrellaCorporation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		json    bool
		opts    *ListOptions
		wantOut []string
	}{
		{
			name: "repo tty",
			tty:  true,
			opts: &ListOptions{},
			wantOut: []string{
				"NAME          UPDATED",
				"SECRET_ONE    about 34 years ago",
				"SECRET_TWO    about 2 years ago",
				"SECRET_THREE  about 47 years ago",
			},
		},
		{
			name: "repo not tty",
			tty:  false,
			opts: &ListOptions{},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z",
				"SECRET_TWO\t2020-12-04T00:00:00Z",
				"SECRET_THREE\t1975-11-30T00:00:00Z",
			},
		},
		{
			name: "org tty",
			tty:  true,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"NAME          UPDATED             VISIBILITY",
				"SECRET_ONE    about 34 years ago  Visible to all repositories",
				"SECRET_TWO    about 2 years ago   Visible to private repositories",
				"SECRET_THREE  about 47 years ago  Visible to 2 selected repositories",
			},
		},
		{
			name: "org not tty",
			tty:  false,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z\tALL",
				"SECRET_TWO\t2020-12-04T00:00:00Z\tPRIVATE",
				"SECRET_THREE\t1975-11-30T00:00:00Z\tSELECTED",
			},
		},
		{
			name: "org tty, json",
			tty:  true,
			json: true,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				// Note the `"numSelectedRepos":2` pair in the last entry.
				`[{"name":"SECRET_ONE","numSelectedRepos":0,"selectedReposURL":"","updatedAt":"1988-10-11T00:00:00Z","visibility":"all"},{"name":"SECRET_TWO","numSelectedRepos":0,"selectedReposURL":"","updatedAt":"2020-12-04T00:00:00Z","visibility":"private"},{"name":"SECRET_THREE","numSelectedRepos":2,"selectedReposURL":"https://api.github.com/orgs/UmbrellaCorporation/actions/secrets/SECRET_THREE/repositories","updatedAt":"1975-11-30T00:00:00Z","visibility":"selected"}]`,
			},
		},
		{
			name: "org not tty, json",
			tty:  false,
			json: true,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				// Note the `"numSelectedRepos":2` pair in the last entry.
				`[{"name":"SECRET_ONE","numSelectedRepos":0,"selectedReposURL":"","updatedAt":"1988-10-11T00:00:00Z","visibility":"all"},{"name":"SECRET_TWO","numSelectedRepos":0,"selectedReposURL":"","updatedAt":"2020-12-04T00:00:00Z","visibility":"private"},{"name":"SECRET_THREE","numSelectedRepos":2,"selectedReposURL":"https://api.github.com/orgs/UmbrellaCorporation/actions/secrets/SECRET_THREE/repositories","updatedAt":"1975-11-30T00:00:00Z","visibility":"selected"}]`,
			},
		},
		{
			name: "env tty",
			tty:  true,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"NAME          UPDATED",
				"SECRET_ONE    about 34 years ago",
				"SECRET_TWO    about 2 years ago",
				"SECRET_THREE  about 47 years ago",
			},
		},
		{
			name: "env not tty",
			tty:  false,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z",
				"SECRET_TWO\t2020-12-04T00:00:00Z",
				"SECRET_THREE\t1975-11-30T00:00:00Z",
			},
		},
		{
			name: "user tty",
			tty:  true,
			opts: &ListOptions{
				UserSecrets: true,
			},
			wantOut: []string{
				"NAME          UPDATED             VISIBILITY",
				"SECRET_ONE    about 34 years ago  Visible to 1 selected repository",
				"SECRET_TWO    about 2 years ago   Visible to 2 selected repositories",
				"SECRET_THREE  about 47 years ago  Visible to 3 selected repositories",
			},
		},
		{
			name: "user not tty",
			tty:  false,
			opts: &ListOptions{
				UserSecrets: true,
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z\tSELECTED",
				"SECRET_TWO\t2020-12-04T00:00:00Z\tSELECTED",
				"SECRET_THREE\t1975-11-30T00:00:00Z\tSELECTED",
			},
		},
		{
			name: "user tty, json",
			tty:  true,
			json: true,
			opts: &ListOptions{
				UserSecrets: true,
			},
			wantOut: []string{
				// Note that `numSelectedRepos` fields are not set to default (zero).
				`[{"name":"SECRET_ONE","numSelectedRepos":1,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_ONE/repositories","updatedAt":"1988-10-11T00:00:00Z","visibility":"selected"},{"name":"SECRET_TWO","numSelectedRepos":2,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_TWO/repositories","updatedAt":"2020-12-04T00:00:00Z","visibility":"selected"},{"name":"SECRET_THREE","numSelectedRepos":3,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_THREE/repositories","updatedAt":"1975-11-30T00:00:00Z","visibility":"selected"}]`,
			},
		},
		{
			name: "user not tty, json",
			tty:  false,
			json: true,
			opts: &ListOptions{
				UserSecrets: true,
			},
			wantOut: []string{
				// Note that `numSelectedRepos` fields are not set to default (zero).
				`[{"name":"SECRET_ONE","numSelectedRepos":1,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_ONE/repositories","updatedAt":"1988-10-11T00:00:00Z","visibility":"selected"},{"name":"SECRET_TWO","numSelectedRepos":2,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_TWO/repositories","updatedAt":"2020-12-04T00:00:00Z","visibility":"selected"},{"name":"SECRET_THREE","numSelectedRepos":3,"selectedReposURL":"https://api.github.com/user/codespaces/secrets/SECRET_THREE/repositories","updatedAt":"1975-11-30T00:00:00Z","visibility":"selected"}]`,
			},
		},
		{
			name: "Dependabot repo tty",
			tty:  true,
			opts: &ListOptions{
				Application: "Dependabot",
			},
			wantOut: []string{
				"NAME          UPDATED",
				"SECRET_ONE    about 34 years ago",
				"SECRET_TWO    about 2 years ago",
				"SECRET_THREE  about 47 years ago",
			},
		},
		{
			name: "Dependabot repo not tty",
			tty:  false,
			opts: &ListOptions{
				Application: "Dependabot",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z",
				"SECRET_TWO\t2020-12-04T00:00:00Z",
				"SECRET_THREE\t1975-11-30T00:00:00Z",
			},
		},
		{
			name: "Dependabot org tty",
			tty:  true,
			opts: &ListOptions{
				Application: "Dependabot",
				OrgName:     "UmbrellaCorporation",
			},
			wantOut: []string{
				"NAME          UPDATED             VISIBILITY",
				"SECRET_ONE    about 34 years ago  Visible to all repositories",
				"SECRET_TWO    about 2 years ago   Visible to private repositories",
				"SECRET_THREE  about 47 years ago  Visible to 2 selected repositories",
			},
		},
		{
			name: "Dependabot org not tty",
			tty:  false,
			opts: &ListOptions{
				Application: "Dependabot",
				OrgName:     "UmbrellaCorporation",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11T00:00:00Z\tALL",
				"SECRET_TWO\t2020-12-04T00:00:00Z\tPRIVATE",
				"SECRET_THREE\t1975-11-30T00:00:00Z\tSELECTED",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Verify(t)

			path := "repos/owner/repo/actions/secrets"
			if tt.opts.EnvName != "" {
				path = fmt.Sprintf("repos/owner/repo/environments/%s/secrets", tt.opts.EnvName)
			} else if tt.opts.OrgName != "" {
				path = fmt.Sprintf("orgs/%s/actions/secrets", tt.opts.OrgName)
			}

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			t1, _ := time.Parse("2006-01-02", "2020-12-04")
			t2, _ := time.Parse("2006-01-02", "1975-11-30")
			payload := struct {
				Secrets []Secret
			}{
				Secrets: []Secret{
					{
						Name:      "SECRET_ONE",
						UpdatedAt: t0,
					},
					{
						Name:      "SECRET_TWO",
						UpdatedAt: t1,
					},
					{
						Name:      "SECRET_THREE",
						UpdatedAt: t2,
					},
				},
			}
			if tt.opts.OrgName != "" {
				payload.Secrets = []Secret{
					{
						Name:       "SECRET_ONE",
						UpdatedAt:  t0,
						Visibility: shared.All,
					},
					{
						Name:       "SECRET_TWO",
						UpdatedAt:  t1,
						Visibility: shared.Private,
					},
					{
						Name:             "SECRET_THREE",
						UpdatedAt:        t2,
						Visibility:       shared.Selected,
						SelectedReposURL: fmt.Sprintf("https://api.github.com/orgs/%s/actions/secrets/SECRET_THREE/repositories", tt.opts.OrgName),
					},
				}

				reg.Register(
					httpmock.REST("GET", fmt.Sprintf("orgs/%s/actions/secrets/SECRET_THREE/repositories", tt.opts.OrgName)),
					httpmock.JSONResponse(struct {
						TotalCount int `json:"total_count"`
					}{2}))
			}

			if tt.opts.UserSecrets {
				payload.Secrets = []Secret{
					{
						Name:             "SECRET_ONE",
						UpdatedAt:        t0,
						Visibility:       shared.Selected,
						SelectedReposURL: "https://api.github.com/user/codespaces/secrets/SECRET_ONE/repositories",
					},
					{
						Name:             "SECRET_TWO",
						UpdatedAt:        t1,
						Visibility:       shared.Selected,
						SelectedReposURL: "https://api.github.com/user/codespaces/secrets/SECRET_TWO/repositories",
					},
					{
						Name:             "SECRET_THREE",
						UpdatedAt:        t2,
						Visibility:       shared.Selected,
						SelectedReposURL: "https://api.github.com/user/codespaces/secrets/SECRET_THREE/repositories",
					},
				}

				path = "user/codespaces/secrets"
				for i, secret := range payload.Secrets {
					hostLen := len("https://api.github.com/")
					path := secret.SelectedReposURL[hostLen:len(secret.SelectedReposURL)]
					repositoryCount := i + 1
					reg.Register(
						httpmock.REST("GET", path),
						httpmock.JSONResponse(struct {
							TotalCount int `json:"total_count"`
						}{repositoryCount}))
				}
			}

			if tt.opts.Application == "Dependabot" {
				path = strings.Replace(path, "actions", "dependabot", 1)
			}

			reg.Register(httpmock.REST("GET", path), httpmock.JSONResponse(payload))

			ios, _, stdout, _ := iostreams.Test()

			ios.SetStdoutTTY(tt.tty)

			tt.opts.IO = ios
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.Now = func() time.Time {
				t, _ := time.Parse(time.RFC822, "15 Mar 23 00:00 UTC")
				return t
			}

			if tt.json {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(secretFields)
				tt.opts.Exporter = exporter
			}

			err := listRun(tt.opts)
			assert.NoError(t, err)

			expected := fmt.Sprintf("%s\n", strings.Join(tt.wantOut, "\n"))
			assert.Equal(t, expected, stdout.String())
		})
	}
}

// Test_listRun_populatesNumSelectedReposIfRequired asserts that NumSelectedRepos
// field is populated **only** when it's going to be presented in the output. Since
// populating this field costs further API requests (one per secret), it's important
// to avoid extra calls when the output will not present the field's value. Note
// that NumSelectedRepos is only meant for user or organization secrets.
//
// We should only populate the NumSelectedRepos field in these cases:
//  1. The command is run in the TTY mode without the `--json <fields>` option.
//  2. The command is run with `--json <fields>` option, and `numSelectedRepos`
//     is among the selected fields. In this case, TTY mode is irrelevant.
func Test_listRun_populatesNumSelectedReposIfRequired(t *testing.T) {
	type secretKind string
	const secretKindUser secretKind = "user"
	const secretKindOrg secretKind = "org"

	tests := []struct {
		name          string
		kind          secretKind
		tty           bool
		jsonFields    []string
		wantPopulated bool
	}{
		{
			name:          "org tty",
			kind:          secretKindOrg,
			tty:           true,
			wantPopulated: true,
		},
		{
			name:          "org tty, json with numSelectedRepos",
			kind:          secretKindOrg,
			tty:           true,
			jsonFields:    []string{"numSelectedRepos"},
			wantPopulated: true,
		},
		{
			name:          "org tty, json without numSelectedRepos",
			kind:          secretKindOrg,
			tty:           true,
			jsonFields:    []string{"name"},
			wantPopulated: false,
		},
		{
			name:          "org not tty",
			kind:          secretKindOrg,
			tty:           false,
			wantPopulated: false,
		},
		{
			name:          "org not tty, json with numSelectedRepos",
			kind:          secretKindOrg,
			tty:           false,
			jsonFields:    []string{"numSelectedRepos"},
			wantPopulated: true,
		},
		{
			name:          "org not tty, json without numSelectedRepos",
			kind:          secretKindOrg,
			tty:           false,
			jsonFields:    []string{"name"},
			wantPopulated: false,
		},
		{
			name:          "user tty",
			kind:          secretKindUser,
			tty:           true,
			wantPopulated: true,
		},
		{
			name:          "user tty, json with numSelectedRepos",
			kind:          secretKindUser,
			tty:           true,
			jsonFields:    []string{"numSelectedRepos"},
			wantPopulated: true,
		},
		{
			name:          "user tty, json without numSelectedRepos",
			kind:          secretKindUser,
			tty:           true,
			jsonFields:    []string{"name"},
			wantPopulated: false,
		},
		{
			name:          "user not tty",
			kind:          secretKindUser,
			tty:           false,
			wantPopulated: false,
		},
		{
			name:          "user not tty, json with numSelectedRepos",
			kind:          secretKindUser,
			tty:           false,
			jsonFields:    []string{"numSelectedRepos"},
			wantPopulated: true,
		},
		{
			name:          "user not tty, json without numSelectedRepos",
			kind:          secretKindUser,
			tty:           false,
			jsonFields:    []string{"name"},
			wantPopulated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Verify(t)

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			opts := &ListOptions{}

			if tt.kind == secretKindOrg {
				opts.OrgName = "umbrellaOrganization"
				reg.Register(
					httpmock.REST("GET", "orgs/umbrellaOrganization/actions/secrets"),
					httpmock.JSONResponse(struct{ Secrets []Secret }{
						[]Secret{
							{
								Name:             "SECRET",
								UpdatedAt:        t0,
								Visibility:       shared.Selected,
								SelectedReposURL: "https://api.github.com/orgs/umbrellaOrganization/actions/secrets/SECRET/repositories",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "orgs/umbrellaOrganization/actions/secrets/SECRET/repositories"),
					httpmock.JSONResponse(struct {
						TotalCount int `json:"total_count"`
					}{999}))
			}

			if tt.kind == secretKindUser {
				opts.UserSecrets = true
				reg.Register(
					httpmock.REST("GET", "user/codespaces/secrets"),
					httpmock.JSONResponse(struct{ Secrets []Secret }{
						[]Secret{
							{
								Name:             "SECRET",
								UpdatedAt:        t0,
								Visibility:       shared.Selected,
								SelectedReposURL: "https://api.github.com/user/codespaces/secrets/SECRET/repositories",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "user/codespaces/secrets/SECRET/repositories"),
					httpmock.JSONResponse(struct {
						TotalCount int `json:"total_count"`
					}{999}))
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			opts.IO = ios

			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			opts.Now = func() time.Time {
				t, _ := time.Parse(time.RFC822, "4 Apr 24 00:00 UTC")
				return t
			}

			err := listRun(opts)
			assert.NoError(t, err)

			if tt.wantPopulated {
				// There should be 2 requests; one to get the secrets list and
				// another to populate the numSelectedRepos field.
				assert.Len(t, reg.Requests, 2)
			} else {
				// Only one requests to get the secrets list.
				assert.Len(t, reg.Requests, 1)
			}
		})
	}
}

func Test_getSecrets_pagination(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "path/to", url.Values{"per_page": []string{"100"}}),
		httpmock.WithHeader(
			httpmock.StringResponse(`{"secrets":[{},{}]}`),
			"Link",
			`<http://example.com/page/0>; rel="previous", <http://example.com/page/2>; rel="next"`),
	)
	reg.Register(
		httpmock.REST("GET", "page/2"),
		httpmock.StringResponse(`{"secrets":[{},{}]}`),
	)
	client := &http.Client{Transport: reg}
	secrets, err := getSecrets(client, "github.com", "path/to")
	assert.NoError(t, err)
	assert.Equal(t, 4, len(secrets))
}

func TestExportSecrets(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	tf, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	ss := []Secret{{
		Name:             "s1",
		UpdatedAt:        tf,
		Visibility:       shared.All,
		SelectedReposURL: "https://someurl.com",
		NumSelectedRepos: 1,
	}}
	exporter := cmdutil.NewJSONExporter()
	exporter.SetFields(secretFields)
	require.NoError(t, exporter.Write(ios, ss))
	require.JSONEq(t,
		`[{"name":"s1","numSelectedRepos":1,"selectedReposURL":"https://someurl.com","updatedAt":"2024-01-01T00:00:00Z","visibility":"all"}]`,
		stdout.String())
}
