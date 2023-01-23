package list

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_NewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants shared.ListOptions
	}{
		{
			name: "repo",
			cli:  "",
			wants: shared.ListOptions{
				OrgName: "",
			},
		},
		{
			name: "org",
			cli:  "-oUmbrellaCorporation",
			wants: shared.ListOptions{
				OrgName: "UmbrellaCorporation",
				Page: 1,
				PerPage: 5,
			},
		},
		{
			name: "env",
			cli:  "-eDevelopment",
			wants: shared.ListOptions{
				EnvName: "Development",
				Page: 1,
				PerPage: 2,
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

			var gotOpts *shared.ListOptions
			cmd := NewCmdList(f, func(opts *shared.ListOptions) error {
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
		opts    *shared.ListOptions
		wantOut []string
	}{
		{
			name: "repo tty",
			tty:  true,
			opts: &shared.ListOptions{ },
			wantOut: []string{
				"VARIABLE_ONE.*Updated 1988-10-11",
				"VARIABLE_TWO.*Updated 2020-12-04",
				"VARIABLE_THREE.*Updated 1975-11-30",
			},
		},
		{
			name: "repo not tty",
			tty:  false,
			opts: &shared.ListOptions{ },
			wantOut: []string{
				"VARIABLE_ONE\tone\t1988-10-11",
				"VARIABLE_TWO\ttwo\t2020-12-04",
				"VARIABLE_THREE\tthree\t1975-11-30",
			},
		},
		{
			name: "org tty",
			tty:  true,
			opts: &shared.ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"VARIABLE_ONE.*Updated 1988-10-11.*Visible to all repositories",
				"VARIABLE_TWO.*Updated 2020-12-04.*Visible to private repositories",
				"VARIABLE_THREE.*Updated 1975-11-30.*Visible to 2 selected repositories",
			},
		},
		{
			name: "org not tty",
			tty:  false,
			opts: &shared.ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"VARIABLE_ONE\torg1\t1988-10-11\tALL",
				"VARIABLE_TWO\torg2\t2020-12-04\tPRIVATE",
				"VARIABLE_THREE\torg3\t1975-11-30\tSELECTED",
			},
		},
		{
			name: "env tty",
			tty:  true,
			opts: &shared.ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"VARIABLE_ONE.*Updated 1988-10-11",
				"VARIABLE_TWO.*Updated 2020-12-04",
				"VARIABLE_THREE.*Updated 1975-11-30",
			},
		},
		{
			name: "env not tty",
			tty:  false,
			opts: &shared.ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"VARIABLE_ONE\tone\t1988-10-11",
				"VARIABLE_TWO\ttwo\t2020-12-04",
				"VARIABLE_THREE\tthree\t1975-11-30",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			path := "repos/owner/repo/actions/variables"
			if tt.opts.EnvName != "" {
				path = fmt.Sprintf("repos/owner/repo/environments/%s/variables", tt.opts.EnvName)
			}

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			t1, _ := time.Parse("2006-01-02", "2020-12-04")
			t2, _ := time.Parse("2006-01-02", "1975-11-30")
			payload := shared.VariablesPayload{}
			payload.Variables = []*shared.Variable{
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
			}
			if tt.opts.OrgName != "" {
				payload.Variables = []*shared.Variable{
					{
						Name:       "VARIABLE_ONE",
						UpdatedAt:  t0,
						Value:      "org1",
						Visibility: shared.All,
					},
					{
						Name:       "VARIABLE_TWO",
						UpdatedAt:  t1,
						Value:      "org2",
						Visibility: shared.Private,
					},
					{
						Name:             "VARIABLE_THREE",
						UpdatedAt:        t2,
						Value: 			  "org3",
						Visibility:       shared.Selected,
						SelectedReposURL: fmt.Sprintf("https://api.github.com/orgs/%s/actions/variables/VARIABLE_THREE/repositories", tt.opts.OrgName),
					},
				}
				path = fmt.Sprintf("orgs/%s/actions/variables", tt.opts.OrgName)

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

		    err := listRun(tt.opts)
			assert.NoError(t, err)

			reg.Verify(t)

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, stdout.String(), tt.wantOut...)
		})
	}
}
