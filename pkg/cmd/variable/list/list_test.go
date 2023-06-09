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
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
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
		opts    *ListOptions
		wantOut []string
	}{
		{
			name: "repo tty",
			tty:  true,
			opts: &ListOptions{},
			wantOut: []string{
				"NAME            VALUE  UPDATED",
				"VARIABLE_ONE    one    about 34 years ago",
				"VARIABLE_TWO    two    about 2 years ago",
				"VARIABLE_THREE  three  about 47 years ago",
			},
		},
		{
			name: "repo not tty",
			tty:  false,
			opts: &ListOptions{},
			wantOut: []string{
				"VARIABLE_ONE\tone\t1988-10-11T00:00:00Z",
				"VARIABLE_TWO\ttwo\t2020-12-04T00:00:00Z",
				"VARIABLE_THREE\tthree\t1975-11-30T00:00:00Z",
			},
		},
		{
			name: "org tty",
			tty:  true,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"NAME            VALUE      UPDATED             VISIBILITY",
				"VARIABLE_ONE    org_one    about 34 years ago  Visible to all repositories",
				"VARIABLE_TWO    org_two    about 2 years ago   Visible to private repositories",
				"VARIABLE_THREE  org_three  about 47 years ago  Visible to 2 selected reposito...",
			},
		},
		{
			name: "org not tty",
			tty:  false,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"VARIABLE_ONE\torg_one\t1988-10-11T00:00:00Z\tALL",
				"VARIABLE_TWO\torg_two\t2020-12-04T00:00:00Z\tPRIVATE",
				"VARIABLE_THREE\torg_three\t1975-11-30T00:00:00Z\tSELECTED",
			},
		},
		{
			name: "env tty",
			tty:  true,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"NAME            VALUE  UPDATED",
				"VARIABLE_ONE    one    about 34 years ago",
				"VARIABLE_TWO    two    about 2 years ago",
				"VARIABLE_THREE  three  about 47 years ago",
			},
		},
		{
			name: "env not tty",
			tty:  false,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"VARIABLE_ONE\tone\t1988-10-11T00:00:00Z",
				"VARIABLE_TWO\ttwo\t2020-12-04T00:00:00Z",
				"VARIABLE_THREE\tthree\t1975-11-30T00:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			path := "repos/owner/repo/actions/variables"
			if tt.opts.EnvName != "" {
				path = fmt.Sprintf("repos/owner/repo/environments/%s/variables", tt.opts.EnvName)
			} else if tt.opts.OrgName != "" {
				path = fmt.Sprintf("orgs/%s/actions/variables", tt.opts.OrgName)
			}

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			t1, _ := time.Parse("2006-01-02", "2020-12-04")
			t2, _ := time.Parse("2006-01-02", "1975-11-30")
			payload := struct {
				Variables []Variable
			}{
				Variables: []Variable{
					{
						Name:      "VARIABLE_ONE",
						Value:     "one",
						UpdatedAt: t0,
					},
					{
						Name:      "VARIABLE_TWO",
						Value:     "two",
						UpdatedAt: t1,
					},
					{
						Name:      "VARIABLE_THREE",
						Value:     "three",
						UpdatedAt: t2,
					},
				},
			}
			if tt.opts.OrgName != "" {
				payload.Variables = []Variable{
					{
						Name:       "VARIABLE_ONE",
						Value:      "org_one",
						UpdatedAt:  t0,
						Visibility: shared.All,
					},
					{
						Name:       "VARIABLE_TWO",
						Value:      "org_two",
						UpdatedAt:  t1,
						Visibility: shared.Private,
					},
					{
						Name:             "VARIABLE_THREE",
						Value:            "org_three",
						UpdatedAt:        t2,
						Visibility:       shared.Selected,
						SelectedReposURL: fmt.Sprintf("https://api.github.com/orgs/%s/actions/variables/VARIABLE_THREE/repositories", tt.opts.OrgName),
					},
				}
				if tt.tty {
					reg.Register(
						httpmock.REST("GET", fmt.Sprintf("orgs/%s/actions/variables/VARIABLE_THREE/repositories", tt.opts.OrgName)),
						httpmock.JSONResponse(struct {
							TotalCount int `json:"total_count"`
						}{2}))
				}
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
			tt.opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.Now = func() time.Time {
				t, _ := time.Parse(time.RFC822, "15 Mar 23 00:00 UTC")
				return t
			}

			err := listRun(tt.opts)
			assert.NoError(t, err)

			expected := fmt.Sprintf("%s\n", strings.Join(tt.wantOut, "\n"))
			assert.Equal(t, expected, stdout.String())
		})
	}
}

func Test_getVariables_pagination(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.QueryMatcher("GET", "path/to", url.Values{"per_page": []string{"100"}}),
		httpmock.WithHeader(
			httpmock.StringResponse(`{"variables":[{},{}]}`),
			"Link",
			`<http://example.com/page/0>; rel="previous", <http://example.com/page/2>; rel="next"`),
	)
	reg.Register(
		httpmock.REST("GET", "page/2"),
		httpmock.StringResponse(`{"variables":[{},{}]}`),
	)
	client := &http.Client{Transport: reg}
	variables, err := getVariables(client, "github.com", "path/to")
	assert.NoError(t, err)
	assert.Equal(t, 4, len(variables))
}
