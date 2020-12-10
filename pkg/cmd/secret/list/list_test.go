package list

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/secret/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
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
				"SECRET_ONE.*Updated 1988-10-11",
				"SECRET_TWO.*Updated 2020-12-04",
				"SECRET_THREE.*Updated 1975-11-30",
			},
		},
		{
			name: "repo not tty",
			tty:  false,
			opts: &ListOptions{},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11",
				"SECRET_TWO\t2020-12-04",
				"SECRET_THREE\t1975-11-30",
			},
		},
		{
			name: "org tty",
			tty:  true,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"SECRET_ONE.*Updated 1988-10-11.*Visible to all repositories",
				"SECRET_TWO.*Updated 2020-12-04.*Visible to private repositories",
				"SECRET_THREE.*Updated 1975-11-30.*Visible to 2 selected repositories",
			},
		},
		{
			name: "org not tty",
			tty:  false,
			opts: &ListOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11\tALL",
				"SECRET_TWO\t2020-12-04\tPRIVATE",
				"SECRET_THREE\t1975-11-30\tSELECTED",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			t1, _ := time.Parse("2006-01-02", "2020-12-04")
			t2, _ := time.Parse("2006-01-02", "1975-11-30")
			path := "repos/owner/repo/actions/secrets"
			payload := secretsPayload{}
			payload.Secrets = []*Secret{
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
			}
			if tt.opts.OrgName != "" {
				payload.Secrets = []*Secret{
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
				path = fmt.Sprintf("orgs/%s/actions/secrets", tt.opts.OrgName)

				reg.Register(
					httpmock.REST("GET", fmt.Sprintf("orgs/%s/actions/secrets/SECRET_THREE/repositories", tt.opts.OrgName)),
					httpmock.JSONResponse(struct {
						TotalCount int `json:"total_count"`
					}{2}))
			}

			reg.Register(httpmock.REST("GET", path), httpmock.JSONResponse(payload))

			io, _, stdout, _ := iostreams.Test()

			io.SetStdoutTTY(tt.tty)

			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			err := listRun(tt.opts)
			assert.NoError(t, err)

			reg.Verify(t)

			test.ExpectLines(t, stdout.String(), tt.wantOut...)
		})
	}
}
