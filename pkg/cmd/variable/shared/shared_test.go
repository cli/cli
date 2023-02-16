package shared

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		supportedEntities   []VariableEntity
		unsupportedEntities []VariableEntity
	}{
		{
			name: "Actions",
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
				assert.True(t, IsSupportedVariableEntity(entity))
			}

			for _, entity := range tt.unsupportedEntities {
				assert.False(t, IsSupportedVariableEntity(entity))
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
