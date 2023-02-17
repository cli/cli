package set

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
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
			wants: SetOptions{
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
			wants: SetOptions{
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
			name: "vis all",
			cli:  `cool_variable --org coolOrg -b"cool" -vall`,
			wants: SetOptions{
				VariableName: "cool_variable",
				Visibility:   shared.All,
				Body:         "cool",
				OrgName:      "coolOrg",
			},
		},
		{
			name: "no store",
			cli:  `cool_variable`,
			wants: SetOptions{
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
			assert.ElementsMatch(t, tt.wants.RepositoryNames, gotOpts.RepositoryNames)
		})
	}
}

func Test_setRun_repo(t *testing.T) {
	tests := []struct {
		name    string
		opts    *SetOptions
		wantApp string
		wantErr bool
	}{
		{
			name:    "Actions",
			opts:    &SetOptions{},
			wantApp: "actions",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			reg.Register(httpmock.REST("POST", fmt.Sprintf("repos/owner/repo/%s/variables", tt.wantApp)),
				httpmock.StatusStringResponse(201, `{}`))

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

func Test_setRun_env(t *testing.T) {
	reg := &httpmock.Registry{}

	reg.Register(httpmock.REST("POST", "repositories/1/environments/development/variables"), httpmock.StatusStringResponse(201, `{}`))
	reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
		httpmock.StringResponse(`{"data":{"repo_0001":{"databaseId":1}}}`))

	ios, _, _, _ := iostreams.Test()

	opts := &SetOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) { return config.NewBlankConfig(), nil },
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		EnvName:      "development",
		IO:           ios,
		VariableName: "cool_variable",
		Body:         "a variable",
	}

	err := setRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)

	data, err := io.ReadAll(reg.Requests[1].Body)
	assert.NoError(t, err)
	var payload shared.VariablePayload
	err = json.Unmarshal(data, &payload)
	assert.NoError(t, err)
	assert.Equal(t, payload.Value, "a variable")
}

func Test_setRun_org(t *testing.T) {
	tests := []struct {
		name             string
		opts             *SetOptions
		wantVisibility   shared.Visibility
		wantRepositories []int64
		wantApp          string
	}{
		{
			name: "all vis",
			opts: &SetOptions{
				OrgName:    "UmbrellaCorporation",
				Visibility: shared.All,
			},
			wantApp: "actions",
		},
		{
			name: "selected visibility",
			opts: &SetOptions{
				OrgName:         "UmbrellaCorporation",
				Visibility:      shared.Selected,
				RepositoryNames: []string{"birkin", "UmbrellaCorporation/wesker"},
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

			err := setRun(tt.opts)
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

func Test_getBodyPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	opts := &SetOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		IO:   ios,
		Body: "a variable",
	}
	httpClient, _ := opts.HttpClient()
	apiClient := api.NewClientFromHTTP(httpClient)

	body, err := getBody(opts, apiClient, "owner/repo", nil)
	assert.NoError(t, err)
	assert.Equal(t, body.Value, "a variable")
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
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			opts := &SetOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				IO:           ios,
				VariableName: "VARNAME",
				Body:         "a variable",
			}
			httpClient, _ := opts.HttpClient()
			apiClient := api.NewClientFromHTTP(httpClient)

			body, err := getBody(opts, apiClient, "owner/repo", nil)
			assert.NoError(t, err)

			assert.Equal(t, tt.want, body.Value)
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
		name       string
		opts       SetOptions
		isTTY      bool
		stdin      string
		want       shared.VariablePayload
		wantErr    bool
		isUpdate   bool
		currentVar shared.VariablePayload
	}{
		{
			name: "variable from arg",
			opts: SetOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				VariableName: "FOO",
				Body:         "bar",
				CsvFile:      "",
			},
			want: shared.VariablePayload{Name: "FOO",
				Value: "bar"},
		},
		{
			name: "variables from file",
			opts: SetOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				Body: "",
				CsvFile: genFile(heredoc.Doc(`
					FOO,bar
					#IGNORED, true
				`)),
			},
			want: shared.VariablePayload{Name: "FOO",
				Value: "bar"},
		},
		{
			name: "update repo variables from file",
			opts: SetOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				Body: "",
				CsvFile: genFile(heredoc.Doc(`
					FOO,bar
					#IGNORED, true
				`)),
			},
			want: shared.VariablePayload{Name: "FOO",
				Value: "bar"},
			isUpdate: true,
			currentVar: shared.VariablePayload{
				Name:  "FOO",
				Value: "abc",
			},
		},
		{
			name: "update org variable from arg",
			opts: SetOptions{
				OrgName:      "org",
				VariableName: "FOO",
				Body:         "bar, all",
				CsvFile:      "",
			},
			isUpdate: true,
			currentVar: shared.VariablePayload{
				Name:  "FOO",
				Value: "abc",
			},
			want: shared.VariablePayload{Name: "FOO",
				Value:      "bar",
				Visibility: shared.All},
		},
		{
			name: "update org variables from file to selected repos",
			opts: SetOptions{
				OrgName: "org",
				Body:    "",
				CsvFile: genFile(heredoc.Doc(`
					FOO,bar,selected,repo1,repo2
					#IGNORED, true
				`)),
			},
			want: shared.VariablePayload{Name: "FOO",
				Value:        "bar",
				Visibility:   shared.Selected,
				Repositories: []int64{1, 2}},
			isUpdate: true,
			currentVar: shared.VariablePayload{
				Name:       "FOO",
				Value:      "abc",
				Visibility: shared.Private,
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
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			apiClient := api.NewClientFromHTTP(&http.Client{Transport: reg})
			var host string

			if tt.opts.OrgName != "" {
				host = "github.com"
				opts.Config = func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				}
				if tt.isUpdate {
					current, err := json.Marshal(tt.currentVar)
					if err != nil {
						reg.Register(httpmock.REST("GET", "orgs/org/actions/variables/FOO"),
							httpmock.StatusStringResponse(200, string(current)))
					}
					if strings.Contains(tt.name, "selected repos") {
						reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
							httpmock.StatusStringResponse(200, `{"data":{"repo_0001":{"databaseId":1},"repo_0002":{"databaseId":2}}}`))
					}
				} else {
					reg.Register(httpmock.REST("GET", "orgs/org/actions/variables/FOO"),
						httpmock.StatusStringResponse(404, "Not Found"))
				}
			} else {
				host = "owner/repo"
				if !tt.isUpdate {
					reg.Register(httpmock.REST("GET", "repos/owner/repo/actions/variables/FOO"),
						httpmock.StatusStringResponse(404, "Not Found"))
				} else {
					current, err := json.Marshal(tt.currentVar)
					if err != nil {
						reg.Register(httpmock.REST("GET", "repos/owner/repo/actions/variables/FOO"),
							httpmock.StatusStringResponse(200, string(current)))
					}
				}
			}

			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			gotVariables, err := getVariablesFromOptions(&opts, apiClient, host)
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("getVariablesFromOptions() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if tt.wantErr {
				t.Fatalf("getVariablesFromOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(gotVariables) != 1 {
				t.Fatalf("getVariablesFromOptions() = got %d variables, want %d", len(gotVariables), 1)
			}
			if tt.want.Name != gotVariables["FOO"].Name {
				t.Errorf("getVariablesFromOptions() got %q, want %q", gotVariables["FOO"].Name, tt.want.Name)
			}
			if tt.want.Value != gotVariables["FOO"].Value {
				t.Errorf("getVariablesFromOptions() got %q, want %q", gotVariables["FOO"].Value, tt.want.Value)
			}
			if tt.want.Visibility != gotVariables["FOO"].Visibility {
				t.Errorf("getVariablesFromOptions() got %q, want %q", gotVariables["FOO"].Visibility, tt.want.Visibility)
			}
			for i := range tt.want.Repositories {
				if tt.want.Repositories[i] != gotVariables["FOO"].Repositories[i] {
					t.Errorf("getVariablesFromOptions() got %q, want %q", gotVariables["FOO"].Repositories[i], tt.want.Repositories[i])
				}
			}
		})
	}
}
