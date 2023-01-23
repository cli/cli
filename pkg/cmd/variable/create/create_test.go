package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    shared.PostPatchOptions
		stdinTTY bool
		wantsErr bool
	}{
		{
			name:     "invalid visibility",
			cli:      "cool_variable --org coolOrg -v'mistyVeil'",
			wantsErr: true,
		},
		{
			name:     "invalid visibility",
			cli:      "cool_variable --org coolOrg -v'selected'",
			wantsErr: true,
		},
		{
			name:     "repos with wrong vis",
			cli:      "cool_variable --org coolOrg -v'private' -rcoolRepo",
			wantsErr: true,
		},
		{
			name:     "no name",
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "multiple names",
			cli:      "cool_variable good_variable",
			wantsErr: true,
		},
		{
			name:     "visibility without org",
			cli:      "cool_variable -vall",
			wantsErr: true,
		},
		{
			name: "repos without vis",
			cli:  "cool_variable -bs --org coolOrg -rcoolRepo",
			wants: shared.PostPatchOptions{
				VariableName:    "cool_variable",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"coolRepo"},
				Body:            "s",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "org with selected repo",
			cli:  "-ocoolOrg -bs -vselected -rcoolRepo cool_variable",
			wants: shared.PostPatchOptions{
				VariableName:    "cool_variable",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"coolRepo"},
				Body:            "s",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "org with selected repos",
			cli:  `--org=coolOrg -bs -vselected -r="coolRepo,radRepo,goodRepo" cool_variable`,
			wants: shared.PostPatchOptions{
				VariableName:    "cool_variable",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"coolRepo", "goodRepo", "radRepo"},
				Body:            "s",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "repo",
			cli:  `cool_variable -b"a variable"`,
			wants: shared.PostPatchOptions{
				VariableName: "cool_variable",
				Visibility:   shared.Private,
				Body:         "a variable",
				OrgName:      "",
			},
		},
		{
			name: "env",
			cli:  `cool_variable -b"a variable" -eRelease`,
			wants: shared.PostPatchOptions{
				VariableName: "cool_variable",
				Visibility:   shared.Private,
				Body:         "a variable",
				OrgName:      "",
				EnvName:      "Release",
			},
		},
		{
			name: "vis all",
			cli:  `cool_variable --org coolOrg -b"cool" -vall`,
			wants: shared.PostPatchOptions{
				VariableName: "cool_variable",
				Visibility:   shared.All,
				Body:         "cool",
				OrgName:      "coolOrg",
			},
		},
		{
			name: "no store",
			cli:  `cool_variable`,
			wants: shared.PostPatchOptions{
				VariableName: "cool_variable",
				Visibility:   shared.Private,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			ios.SetStdinTTY(tt.stdinTTY)

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *shared.PostPatchOptions
			cmd := NewCmdCreate(f, func(opts *shared.PostPatchOptions) error {
				gotOpts = opts
				return nil
			})
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

			assert.Equal(t, tt.wants.VariableName, gotOpts.VariableName)
			assert.Equal(t, tt.wants.Body, gotOpts.Body)
			assert.Equal(t, tt.wants.Visibility, gotOpts.Visibility)
			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
			assert.ElementsMatch(t, tt.wants.RepositoryNames, gotOpts.RepositoryNames)
		})
	}
}

func Test_createRun_repo(t *testing.T) {
	tests := []struct {
		name    string
		opts    *shared.PostPatchOptions
		wantApp string
		wantErr bool
	}{
		{
			name: "Actions",
			opts: &shared.PostPatchOptions{
				Application: "actions",
			},
			wantApp: "actions",
			wantErr: false,
		},
		{
			name: "defaults to unknown",
			opts: &shared.PostPatchOptions{
				Application: "",
			},
			wantApp: "unknown",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			reg.Register(httpmock.REST("POST", fmt.Sprintf("repos/owner/repo/%s/variables", tt.wantApp)),
				httpmock.StatusStringResponse(201, `{}`))

			ios, _, _, _ := iostreams.Test()

			opts := &shared.PostPatchOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (config.Config, error) { return config.NewBlankConfig(), nil },
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				IO:             ios,
				VariableName:   "cool_variable",
				Body:           "a variable",
				RandomOverride: fakeRandom,
				Application:    tt.opts.Application,
			}

			err := createRun(opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else{
				assert.NoError(t, err)
			}

			reg.Verify(t)

			data, err := io.ReadAll(reg.Requests[0].Body)
			assert.NoError(t, err)

			var payload shared.VariablePayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.Value, "a variable")
		})
	}
}

func Test_createRun_env(t *testing.T) {
	reg := &httpmock.Registry{}

	reg.Register(httpmock.REST("POST", "repos/owner/repo/environments/development/variables"), httpmock.StatusStringResponse(201, `{}`))

	ios, _, _, _ := iostreams.Test()

	opts := &shared.PostPatchOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) { return config.NewBlankConfig(), nil },
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		EnvName:        "development",
		IO:             ios,
		VariableName:   "cool_variable",
		Body:           "a variable",
		Application:    "actions",
		RandomOverride: fakeRandom,
	}

	err := createRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)

	data, err := io.ReadAll(reg.Requests[0].Body)
	assert.NoError(t, err)
	var payload shared.VariablePayload
	err = json.Unmarshal(data, &payload)
	assert.NoError(t, err)
	assert.Equal(t, payload.Value, "a variable")
}

func Test_createRun_org(t *testing.T) {
	tests := []struct {
		name             string
		opts             *shared.PostPatchOptions
		wantVisibility   shared.Visibility
		wantRepositories []int64
		wantApp          string
	}{
		{
			name: "all vis",
			opts: &shared.PostPatchOptions{
				OrgName:    "UmbrellaCorporation",
				Visibility: shared.All,
				Application: "actions",
			},
			wantApp: "actions",
		},
		{
			name: "selected visibility",
			opts: &shared.PostPatchOptions{
				OrgName:         "UmbrellaCorporation",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"birkin", "UmbrellaCorporation/wesker"},
				Application: "actions",
			},
			wantRepositories: []int64{1, 2},
			wantApp:          "actions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			orgName := tt.opts.OrgName

			reg.Register(httpmock.REST("POST",
				fmt.Sprintf("orgs/%s/%s/variables", orgName, tt.wantApp)),
				httpmock.StatusStringResponse(201, `{}`))

			if len(tt.opts.RepositoryNames) > 0 {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1},"repo_0002":{"databaseId":2}}}`))
			}

			ios, _, _, _ := iostreams.Test()

			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.IO = ios
			tt.opts.VariableName = "org_variable"
			tt.opts.Body = "a variable"
			tt.opts.RandomOverride = fakeRandom

			err := createRun(tt.opts)
			assert.NoError(t, err)

			reg.Verify(t)

			data, err := io.ReadAll(reg.Requests[len(reg.Requests)-1].Body)
			assert.NoError(t, err)
			var payload shared.VariablePayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.Value, "a variable")
			assert.Equal(t, payload.Name, tt.opts.VariableName)
			assert.Equal(t, payload.Visibility, tt.opts.Visibility)
			assert.ElementsMatch(t, payload.Repositories, tt.wantRepositories)
		})
	}
}

func fakeRandom() io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{5}, 32))
}
