package shared

import (
	"bytes"
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
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_getBodyPrompt(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	opts := &PostPatchOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		IO:             ios,
		Body:           "a variable",
		RandomOverride: fakeRandom,
	}
	httpClient, _ := opts.HttpClient()
	apiClient := api.NewClientFromHTTP(httpClient)

	body, err := getBody(opts, apiClient, "owner/repo", false)
	assert.NoError(t, err)
	assert.Equal(t, body.Value, "a variable")
}

func TestGetVariableEntity(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
		envName string
		want    VariableEntity
		wantErr bool
	}{
		{
			name:    "org",
			orgName: "myOrg",
			want:    Organization,
		},
		{
			name:    "env",
			envName: "myEnv",
			want:    Environment,
		},
		{
			name: "defaults to repo",
			want: Repository,
		},
		{
			name:    "Errors if both org and env are set",
			orgName: "myOrg",
			envName: "myEnv",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := GetVariableEntity(tt.orgName, tt.envName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, entity)
			}
		})
	}
}

func TestIsSupportedVariableEntity(t *testing.T) {
	tests := []struct {
		name                string
		app                 App
		supportedEntities   []VariableEntity
		unsupportedEntities []VariableEntity
	}{
		{
			name: "Actions",
			app:  Actions,
			supportedEntities: []VariableEntity{
				Repository,
				Organization,
				Environment,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, entity := range tt.supportedEntities {
				assert.True(t, IsSupportedVariableEntity(tt.app, entity))
			}

			for _, entity := range tt.unsupportedEntities {
				assert.False(t, IsSupportedVariableEntity(tt.app, entity))
			}
		})
	}
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

			opts := &PostPatchOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				IO:             ios,
				VariableName:   "VARNAME",
				Body:           "a variable",
				RandomOverride: fakeRandom,
			}
			httpClient, _ := opts.HttpClient()
			apiClient := api.NewClientFromHTTP(httpClient)

			body, err := getBody(opts, apiClient, "owner/repo", false)
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
		name    string
		opts    PostPatchOptions
		isTTY   bool
		stdin   string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "variable from arg",
			opts: PostPatchOptions{
				VariableName: "FOO",
				Body:         "bar",
				CsvFile:      "",
			},
			want: map[string]string{"FOO": "bar"},
		},
		{
			name: "variables from stdin",
			opts: PostPatchOptions{
				Body:    "",
				CsvFile: "-",
			},
			stdin: `FOO,bar`,
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name: "variables from file",
			opts: PostPatchOptions{
				Body: "",
				CsvFile: genFile(heredoc.Doc(`
					FOO,bar
					QUOTED,my value
					#IGNORED, true
				`)),
			},
			stdin: `FOO=bar`,
			want: map[string]string{
				"FOO":    "bar",
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
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			apiClient := api.NewClientFromHTTP(&http.Client{Transport: reg})

			gotVariables, err := GetVariablesFromOptions(&opts, apiClient, "owner/repo", false)
			if err != nil {
				if !tt.wantErr {
					t.Fatalf("getVariablesFromOptions() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if tt.wantErr {
				t.Fatalf("getVariablesFromOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(gotVariables) != len(tt.want) {
				t.Fatalf("getVariablesFromOptions() = got %d variables, want %d", len(gotVariables), len(tt.want))
			}
			for k, v := range gotVariables {
				if tt.want[k] != v.Value {
					t.Errorf("getVariablesFromOptions() %s = got %q, want %q", k, v.Value, tt.want[k])
				}
			}
		})
	}
}

type testClient func(*http.Request) (*http.Response, error)

func (c testClient) Do(req *http.Request) (*http.Response, error) {
	return c(req)
}

func Test_getVariables_pagination(t *testing.T) {
	var requests []*http.Request
	var client testClient = func(req *http.Request) (*http.Response, error) {
		header := make(map[string][]string)
		if len(requests) == 0 {
			header["Link"] = []string{}
		}
		requests = append(requests, req)
		return &http.Response{
			Request: req,
			Body:    io.NopCloser(strings.NewReader(`{"variables":[{},{}]}`)),
			Header:  header,
		}, nil
	}
	page, perPage := 2, 25
	variables, err := getVariables(client, "github.com", "path/to", page, perPage, "")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(requests))
	assert.Equal(t, 2, len(variables))
	assert.Equal(t, fmt.Sprintf("https://api.github.com/path/to?page=%d&per_page=%d", page, perPage), requests[0].URL.String())
}

func fakeRandom() io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{5}, 32))
}

