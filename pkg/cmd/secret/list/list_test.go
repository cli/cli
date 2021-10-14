package list

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
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
		{
			name: "env tty",
			tty:  true,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"SECRET_ONE.*Updated 1988-10-11",
				"SECRET_TWO.*Updated 2020-12-04",
				"SECRET_THREE.*Updated 1975-11-30",
			},
		},
		{
			name: "env not tty",
			tty:  false,
			opts: &ListOptions{
				EnvName: "Development",
			},
			wantOut: []string{
				"SECRET_ONE\t1988-10-11",
				"SECRET_TWO\t2020-12-04",
				"SECRET_THREE\t1975-11-30",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			path := "repos/owner/repo/actions/secrets"
			if tt.opts.EnvName != "" {
				path = fmt.Sprintf("repos/owner/repo/environments/%s/secrets", tt.opts.EnvName)
			}

			t0, _ := time.Parse("2006-01-02", "1988-10-11")
			t1, _ := time.Parse("2006-01-02", "2020-12-04")
			t2, _ := time.Parse("2006-01-02", "1975-11-30")
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

func Test_getSecrets_pagination(t *testing.T) {
	var requests []*http.Request
	var client testClient = func(req *http.Request) (*http.Response, error) {
		header := make(map[string][]string)
		if len(requests) == 0 {
			header["Link"] = []string{`<http://example.com/page/0>; rel="previous", <http://example.com/page/2>; rel="next"`}
		}
		requests = append(requests, req)
		return &http.Response{
			Request: req,
			Body:    ioutil.NopCloser(strings.NewReader(`{"secrets":[{},{}]}`)),
			Header:  header,
		}, nil
	}

	secrets, err := getSecrets(client, "github.com", "path/to")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(requests))
	assert.Equal(t, 4, len(secrets))
	assert.Equal(t, "https://api.github.com/path/to?per_page=100", requests[0].URL.String())
	assert.Equal(t, "http://example.com/page/2", requests[1].URL.String())
}

type testClient func(*http.Request) (*http.Response, error)

func (c testClient) Do(req *http.Request) (*http.Response, error) {
	return c(req)
}
