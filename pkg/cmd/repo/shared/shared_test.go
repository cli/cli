package shared

import (
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestNewRepo(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	type input struct {
		s      string
		config func() (config.Config, error)
		client *api.Client
	}

	tests := []struct {
		name      string
		input     input
		override  string
		wantName  string
		wantOwner string
		wantHost  string
		wantErr   bool
	}{
		{
			name: "config returns error",
			input: input{s: "REPO",
				config: func() (config.Config, error) { return nil, errors.New("error") },
				client: successClient()},
			wantErr: true,
		},
		{
			name: "client returns error",
			input: input{s: "REPO",
				config: defaultConfig(),
				client: errorClient()},
			wantErr: true,
		},
		{
			name: "config is nil",
			input: input{s: "REPO",
				config: nil,
				client: successClient()},
			wantName:  "REPO",
			wantOwner: "OWNER",
			wantHost:  "github.com",
		},
		{
			name: "config is nil and GH_HOST is set",
			input: input{s: "SOMEONE/REPO",
				config: nil,
				client: successClient()},
			override:  "test.com",
			wantName:  "REPO",
			wantOwner: "SOMEONE",
			wantHost:  "github.com",
		},
		{
			name: "client is nil",
			input: input{s: "REPO",
				config: defaultConfig(),
				client: nil,
			},
			wantName: "REPO",
			wantHost: "nonsense.com",
		},
		{
			name: "REPO returns proper values",
			input: input{s: "REPO",
				config: defaultConfig(),
				client: successClient()},
			wantName:  "REPO",
			wantOwner: "OWNER",
			wantHost:  "nonsense.com",
		},
		{
			name: "SOMEONE/REPO returns proper values",
			input: input{s: "SOMEONE/REPO",
				config: defaultConfig(),
				client: successClient()},
			wantName:  "REPO",
			wantOwner: "SOMEONE",
			wantHost:  "nonsense.com",
		},
		{
			name: "SOMEONE/REPO returns proper values when GH_HOST is set",
			input: input{s: "SOMEONE/REPO",
				config: defaultConfig(),
				client: successClient()},
			override:  "test.com",
			wantName:  "REPO",
			wantOwner: "SOMEONE",
			wantHost:  "test.com",
		},
		{
			name: "HOST/SOMEONE/REPO returns proper values",
			input: input{s: "HOST/SOMEONE/REPO",
				config: defaultConfig(),
				client: successClient()},
			wantName:  "REPO",
			wantOwner: "SOMEONE",
			wantHost:  "host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.override != "" {
				os.Setenv("GH_HOST", tt.override)
			} else {
				os.Unsetenv("GH_HOST")
			}
			r, err := NewRepo(tt.input.s, tt.input.config, tt.input.client)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantName, r.RepoName())
			assert.Equal(t, tt.wantOwner, r.RepoOwner())
			assert.Equal(t, tt.wantHost, r.RepoHost())
		})
	}
}

func defaultConfig() func() (config.Config, error) {
	return func() (config.Config, error) {
		return config.InheritEnv(config.NewFromString(heredoc.Doc(`
      hosts:
        nonsense.com:
          oauth_token: BLAH
		`))), nil
	}
}

func errorClient() *api.Client {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.GraphQL(`query UserCurrent`),
		httpmock.StatusStringResponse(404, "not found"),
	)
	httpClient := &http.Client{Transport: reg}
	apiClient := api.NewClientFromHTTP(httpClient)
	return apiClient
}

func successClient() *api.Client {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.GraphQL(`query UserCurrent`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"OWNER"}}}`),
	)
	httpClient := &http.Client{Transport: reg}
	apiClient := api.NewClientFromHTTP(httpClient)
	return apiClient
}
