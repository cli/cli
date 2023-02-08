package shared

import (
	"bytes"
	"io"
	"net/http"
	"testing"

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
		IO:   ios,
		Body: "a variable",
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
				IO:           ios,
				VariableName: "VARNAME",
				Body:         "a variable",
			}
			httpClient, _ := opts.HttpClient()
			apiClient := api.NewClientFromHTTP(httpClient)

			body, err := getBody(opts, apiClient, "owner/repo", false)
			assert.NoError(t, err)

			assert.Equal(t, tt.want, body.Value)
		})
	}
}

type testClient func(*http.Request) (*http.Response, error)

func (c testClient) Do(req *http.Request) (*http.Response, error) {
	return c(req)
}

func fakeRandom() io.Reader {
	return bytes.NewReader(bytes.Repeat([]byte{5}, 32))
}
