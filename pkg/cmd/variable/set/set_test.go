package set

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdSet(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    SetOptions
		stdinTTY bool
		wantsErr bool
	}{
		{
			name:     "invalid visibility type",
			cli:      "cool_variable --org coolOrg -v'mistyVeil'",
			wantsErr: true,
		},
		{
			name:     "selected visibility with no repos selected",
			cli:      "cool_variable --org coolOrg -v'selected'",
			wantsErr: true,
		},
		{
			name:     "private visibility with repos selected",
			cli:      "cool_variable --org coolOrg -v'private' -rcoolRepo",
			wantsErr: true,
		},
		{
			name:     "no name specified",
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "multiple names specified",
			cli:      "cool_variable good_variable",
			wantsErr: true,
		},
		{
			name:     "visibility without org",
			cli:      "cool_variable -vall",
			wantsErr: true,
		},
		{
			name: "repos without explicit visibility",
			cli:  "cool_variable -bs --org coolOrg -rcoolRepo",
			wants: SetOptions{
				VariableName:    "cool_variable",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"coolRepo"},
				Body:            "s",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "org with explicit visibility and selected repo",
			cli:  "-ocoolOrg -bs -vselected -rcoolRepo cool_variable",
			wants: SetOptions{
				VariableName:    "cool_variable",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"coolRepo"},
				Body:            "s",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "org with explicit visibility and selected repos",
			cli:  `--org=coolOrg -bs -vselected -r="coolRepo,radRepo,goodRepo" cool_variable`,
			wants: SetOptions{
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
			wants: SetOptions{
				VariableName: "cool_variable",
				Visibility:   shared.Private,
				Body:         "a variable",
				OrgName:      "",
			},
		},
		{
			name: "env",
			cli:  `cool_variable -b"a variable" -eRelease`,
			wants: SetOptions{
				VariableName: "cool_variable",
				Visibility:   shared.Private,
				Body:         "a variable",
				OrgName:      "",
				EnvName:      "Release",
			},
		},
		{
			name: "visibility all",
			cli:  `cool_variable --org coolOrg -b"cool" -vall`,
			wants: SetOptions{
				VariableName: "cool_variable",
				Visibility:   shared.All,
				Body:         "cool",
				OrgName:      "coolOrg",
			},
		},
		{
			name: "env file",
			cli:  `--env-file test.env`,
			wants: SetOptions{
				Visibility: shared.Private,
				EnvFile:    "test.env",
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

			var gotOpts *SetOptions
			cmd := NewCmdSet(f, func(opts *SetOptions) error {
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
			assert.Equal(t, tt.wants.EnvFile, gotOpts.EnvFile)
			assert.ElementsMatch(t, tt.wants.RepositoryNames, gotOpts.RepositoryNames)
		})
	}
}

func Test_setRun_repo(t *testing.T) {
	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		wantErr   bool
	}{
		{
			name: "create actions variable",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/owner/repo/actions/variables"),
					httpmock.StatusStringResponse(201, `{}`))
			},
		},
		{
			name: "update actions variable",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/owner/repo/actions/variables"),
					httpmock.StatusStringResponse(409, `{}`))
				reg.Register(httpmock.REST("PATCH", "repos/owner/repo/actions/variables/cool_variable"),
					httpmock.StatusStringResponse(204, `{}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			ios, _, _, _ := iostreams.Test()

			opts := &SetOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (config.Config, error) { return config.NewBlankConfig(), nil },
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				IO:           ios,
				VariableName: "cool_variable",
				Body:         "a variable",
			}

			err := setRun(opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}

			data, err := io.ReadAll(reg.Requests[len(reg.Requests)-1].Body)
			assert.NoError(t, err)

			var payload setPayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.Value, "a variable")
		})
	}
}

func Test_setRun_env(t *testing.T) {
	tests := []struct {
		name      string
		opts      *SetOptions
		httpStubs func(*httpmock.Registry)
		wantErr   bool
	}{
		{
			name: "create env variable",
			opts: &SetOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1}}}`))
				reg.Register(httpmock.REST("POST", "repositories/1/environments/release/variables"),
					httpmock.StatusStringResponse(201, `{}`))
			},
		},
		{
			name: "update env variable",
			opts: &SetOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1}}}`))
				reg.Register(httpmock.REST("POST", "repositories/1/environments/release/variables"),
					httpmock.StatusStringResponse(409, `{}`))
				reg.Register(httpmock.REST("PATCH", "repositories/1/environments/release/variables/cool_variable"),
					httpmock.StatusStringResponse(204, `{}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			ios, _, _, _ := iostreams.Test()

			opts := &SetOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (config.Config, error) { return config.NewBlankConfig(), nil },
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				IO:           ios,
				VariableName: "cool_variable",
				Body:         "a variable",
				EnvName:      "release",
			}

			err := setRun(opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}

			data, err := io.ReadAll(reg.Requests[len(reg.Requests)-1].Body)
			assert.NoError(t, err)

			var payload setPayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.Value, "a variable")
		})
	}
}

func Test_setRun_org(t *testing.T) {
	tests := []struct {
		name             string
		opts             *SetOptions
		httpStubs        func(*httpmock.Registry)
		wantVisibility   shared.Visibility
		wantRepositories []int64
	}{
		{
			name: "create org variable",
			opts: &SetOptions{
				OrgName:    "UmbrellaCorporation",
				Visibility: shared.All,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "orgs/UmbrellaCorporation/actions/variables"),
					httpmock.StatusStringResponse(201, `{}`))
			},
		},
		{
			name: "update org variable",
			opts: &SetOptions{
				OrgName:    "UmbrellaCorporation",
				Visibility: shared.All,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "orgs/UmbrellaCorporation/actions/variables"),
					httpmock.StatusStringResponse(409, `{}`))
				reg.Register(httpmock.REST("PATCH", "orgs/UmbrellaCorporation/actions/variables/cool_variable"),
					httpmock.StatusStringResponse(204, `{}`))
			},
		},
		{
			name: "create org variable with selected visibility",
			opts: &SetOptions{
				OrgName:         "UmbrellaCorporation",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"birkin", "UmbrellaCorporation/wesker"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1},"repo_0002":{"databaseId":2}}}`))
				reg.Register(httpmock.REST("POST", "orgs/UmbrellaCorporation/actions/variables"),
					httpmock.StatusStringResponse(201, `{}`))
			},
			wantRepositories: []int64{1, 2},
		},
		{
			name: "update org variable with selected visibility",
			opts: &SetOptions{
				OrgName:         "UmbrellaCorporation",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"birkin", "UmbrellaCorporation/wesker"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1},"repo_0002":{"databaseId":2}}}`))
				reg.Register(httpmock.REST("POST", "orgs/UmbrellaCorporation/actions/variables"),
					httpmock.StatusStringResponse(409, `{}`))
				reg.Register(httpmock.REST("PATCH", "orgs/UmbrellaCorporation/actions/variables/cool_variable"),
					httpmock.StatusStringResponse(204, `{}`))
			},
			wantRepositories: []int64{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
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
			tt.opts.VariableName = "cool_variable"
			tt.opts.Body = "a variable"

			err := setRun(tt.opts)
			assert.NoError(t, err)

			data, err := io.ReadAll(reg.Requests[len(reg.Requests)-1].Body)
			assert.NoError(t, err)
			var payload setPayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.Value, "a variable")
			assert.Equal(t, payload.Visibility, tt.opts.Visibility)
			assert.ElementsMatch(t, payload.Repositories, tt.wantRepositories)
		})
	}
}

func Test_getBody_prompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	prompterMock := &prompter.PrompterMock{}
	prompterMock.InputFunc = func(message, defaultValue string) (string, error) {
		return "a variable", nil
	}
	opts := &SetOptions{
		IO:       ios,
		Prompter: prompterMock,
	}
	body, err := getBody(opts)
	assert.NoError(t, err)
	assert.Equal(t, "a variable", body)
}

func Test_getBody(t *testing.T) {
	tests := []struct {
		name    string
		bodyArg string
		want    string
		stdin   string
	}{
		{
			name:    "literal value",
			bodyArg: "a variable",
			want:    "a variable",
		},
		{
			name:  "from stdin",
			want:  "a variable",
			stdin: "a variable",
		},
		{
			name:  "from stdin with trailing newline character",
			want:  "a variable",
			stdin: "a variable\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdinTTY(false)
			_, err := stdin.WriteString(tt.stdin)
			assert.NoError(t, err)
			opts := &SetOptions{
				IO:   ios,
				Body: tt.bodyArg,
			}
			body, err := getBody(opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, body)
		})
	}
}

func Test_getVariablesFromOptions(t *testing.T) {
	genFile := func(s string) string {
		f, err := os.CreateTemp("", "gh-env.*")
		if err != nil {
			t.Fatal(err)
			return ""
		}
		defer f.Close()
		t.Cleanup(func() {
			_ = os.Remove(f.Name())
		})
		_, err = f.WriteString(s)
		if err != nil {
			t.Fatal(err)
		}
		return f.Name()
	}

	tests := []struct {
		name    string
		opts    SetOptions
		isTTY   bool
		stdin   string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "variable from arg",
			opts: SetOptions{
				VariableName: "FOO",
				Body:         "bar",
			},
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "variable from stdin",
			opts: SetOptions{
				Body:    "",
				EnvFile: "-",
			},
			stdin: `FOO=bar`,
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name: "variables from file",
			opts: SetOptions{
				Body: "",
				EnvFile: genFile(heredoc.Doc(`
					FOO=bar
					QUOTED="my value"
					#IGNORED=true
					export SHELL=bash
				`)),
			},
			stdin: `FOO=bar`,
			want: map[string]string{
				"FOO":    "bar",
				"SHELL":  "bash",
				"QUOTED": "my value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			stdin.WriteString(tt.stdin)
			opts := tt.opts
			opts.IO = ios
			gotVariables, err := getVariablesFromOptions(&opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, len(tt.want), len(gotVariables))
			for k, v := range gotVariables {
				assert.Equal(t, tt.want[k], v)
			}
		})
	}
}
