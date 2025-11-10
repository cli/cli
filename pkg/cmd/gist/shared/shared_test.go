package shared

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_GetGistIDFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "url",
			url:  "https://gist.github.com/1234",
			want: "1234",
		},
		{
			name: "url with username",
			url:  "https://gist.github.com/octocat/1234",
			want: "1234",
		},
		{
			name: "url, specific file",
			url:  "https://gist.github.com/1234#file-test-md",
			want: "1234",
		},
		{
			name:    "invalid url",
			url:     "https://gist.github.com",
			wantErr: true,
			want:    "Invalid gist URL https://gist.github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := GistIDFromURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.want)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.want, id)
		})
	}
}

func TestIsBinaryContents(t *testing.T) {
	tests := []struct {
		fileContent []byte
		want        bool
	}{
		{
			want:        false,
			fileContent: []byte("package main"),
		},
		{
			want:        false,
			fileContent: []byte(""),
		},
		{
			want:        false,
			fileContent: []byte(nil),
		},
		{
			want: true,
			fileContent: []byte{239, 191, 189, 239, 191, 189, 239, 191, 189, 239,
				191, 189, 239, 191, 189, 16, 74, 70, 73, 70, 239, 191, 189, 1, 1, 1,
				1, 44, 1, 44, 239, 191, 189, 239, 191, 189, 239, 191, 189, 239, 191,
				189, 239, 191, 189, 67, 239, 191, 189, 8, 6, 6, 7, 6, 5, 8, 7, 7, 7,
				9, 9, 8, 10, 12, 20, 10, 12, 11, 11, 12, 25, 18, 19, 15, 20, 29, 26,
				31, 30, 29, 26, 28, 28, 32, 36, 46, 39, 32, 34, 44, 35, 28, 28, 40,
				55, 41, 44, 48, 49, 52, 52, 52, 31, 39, 57, 61, 56, 50, 60, 46, 51,
				52, 50, 239, 191, 189, 239, 191, 189, 239, 191, 189, 67, 1, 9, 9, 9, 12},
		},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, IsBinaryContents(tt.fileContent))
	}
}

func TestPromptGists(t *testing.T) {
	sixHours, _ := time.ParseDuration("6h")
	sixHoursAgo := time.Now().Add(-sixHours)
	sixHoursAgoFormatted := sixHoursAgo.Format(time.RFC3339Nano)

	tests := []struct {
		name          string
		prompterStubs func(pm *prompter.MockPrompter)
		response      string
		wantOut       Gist
		wantErr       bool
	}{
		{
			name: "multiple files, select first gist",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a gist",
					[]string{"cool.txt  about 6 hours ago", "gistfile0.txt  about 6 hours ago"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "cool.txt  about 6 hours ago")
					})
			},
			response: `{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "5678",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
			wantOut: Gist{ID: "1234", Files: map[string]*GistFile{"cool.txt": {Filename: "cool.txt"}}, UpdatedAt: sixHoursAgo, Public: true},
		},
		{
			name: "multiple files, select second gist",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a gist",
					[]string{"cool.txt  about 6 hours ago", "gistfile0.txt  about 6 hours ago"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "gistfile0.txt  about 6 hours ago")
					})
			},
			response: `{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "5678",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
			wantOut: Gist{ID: "5678", Files: map[string]*GistFile{"gistfile0.txt": {Filename: "gistfile0.txt"}}, UpdatedAt: sixHoursAgo, Public: true},
		},
		{
			name:     "no files",
			response: `{ "data": { "viewer": { "gists": { "nodes": [] } } } }`,
			wantOut:  Gist{},
		},
		{
			name: "prompt list contains no-file gist (#10626)",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a gist",
					[]string{"  about 6 hours ago", "gistfile0.txt  about 6 hours ago"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "  about 6 hours ago")
					})
			},
			response: `{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							},
							{
								"name": "5678",
								"files": [{ "name": "gistfile0.txt" }],
								"description": "",
								"updatedAt": "%[1]v",
								"isPublic": true
							}
						] } } } }`,
			wantOut: Gist{ID: "1234", Files: map[string]*GistFile{}, UpdatedAt: sixHoursAgo, Public: true},
		},
	}

	ios, _, _, _ := iostreams.Test()

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		const query = `query GistList\b`
		reg.Register(
			httpmock.GraphQL(query),
			httpmock.StringResponse(fmt.Sprintf(
				tt.response,
				sixHoursAgoFormatted,
			)),
		)
		client := &http.Client{Transport: reg}

		t.Run(tt.name, func(t *testing.T) {
			mockPrompter := prompter.NewMockPrompter(t)
			if tt.prompterStubs != nil {
				tt.prompterStubs(mockPrompter)
			}

			gist, err := PromptGists(mockPrompter, client, "github.com", ios.ColorScheme())
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut.ID, gist.ID)
			reg.Verify(t)
		})
	}
}

func TestGetRawGistFile(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		statusCode  int
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:       "successful request",
			response:   "Hello, World!",
			statusCode: http.StatusOK,
			want:       "Hello, World!",
			wantErr:    false,
		},
		{
			name:       "empty response",
			response:   "",
			statusCode: http.StatusOK,
			want:       "",
			wantErr:    false,
		},
		{
			name:        "not found error",
			response:    "Not Found",
			statusCode:  http.StatusNotFound,
			want:        "",
			wantErr:     true,
			errContains: "HTTP 404",
		},
		{
			name:        "server error",
			response:    "Internal Server Error",
			statusCode:  http.StatusInternalServerError,
			want:        "",
			wantErr:     true,
			errContains: "HTTP 500",
		},
		{
			name:       "large content",
			response:   "This is a very large file content with multiple lines\nLine 2\nLine 3\nAnd more content...",
			statusCode: http.StatusOK,
			want:       "This is a very large file content with multiple lines\nLine 2\nLine 3\nAnd more content...",
			wantErr:    false,
		},
		{
			name:       "special characters",
			response:   "Special chars: àáâãäåæçèéêë 中文 🎉 \"quotes\" 'single'",
			statusCode: http.StatusOK,
			want:       "Special chars: àáâãäåæçèéêë 中文 🎉 \"quotes\" 'single'",
			wantErr:    false,
		},
		{
			name:       "JSON content",
			response:   `{"name": "test", "version": "1.0.0", "dependencies": {"lodash": "^4.17.21"}}`,
			statusCode: http.StatusOK,
			want:       `{"name": "test", "version": "1.0.0", "dependencies": {"lodash": "^4.17.21"}}`,
			wantErr:    false,
		},
		{
			name:       "HTML content",
			response:   "<!DOCTYPE html><html><head><title>Test</title></head><body><h1>Hello</h1></body></html>",
			statusCode: http.StatusOK,
			want:       "<!DOCTYPE html><html><head><title>Test</title></head><body><h1>Hello</h1></body></html>",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.REST("GET", "raw-url"),
				httpmock.StatusStringResponse(tt.statusCode, tt.response),
			)

			client := &http.Client{Transport: reg}
			result, err := GetRawGistFile(client, "https://gist.githubusercontent.com/raw-url")

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}

			reg.Verify(t)
		})
	}
}
