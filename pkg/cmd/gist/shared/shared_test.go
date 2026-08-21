package shared

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		name        string
		fileContent []byte
		want        bool
	}{
		{
			name:        "ASCII text",
			fileContent: []byte("package main"),
			want:        false,
		},
		{
			name:        "empty",
			fileContent: []byte(""),
			want:        false,
		},
		{
			name:        "nil",
			fileContent: []byte(nil),
			want:        false,
		},
		{
			name:        "multi-byte UTF-8",
			fileContent: []byte("café 👋 日本語"),
			want:        false,
		},
		{
			// https://github.com/cli/cli/issues/9761
			name:        "control character in the middle",
			fileContent: []byte("hello\x01world"),
			want:        false,
		},
		{
			name:        "control character at the start",
			fileContent: []byte("\x01hello"),
			want:        false,
		},
		{
			name:        "control character at the end",
			fileContent: []byte("hello\x01"),
			want:        false,
		},
		{
			name:        "every ASCII control character",
			fileContent: allASCIIControlChars(),
			want:        false,
		},
		{
			// Invalid UTF-8 is silently rewritten to U+FFFD when encoded as
			// JSON, so it must not be uploaded.
			name:        "latin-1 text",
			fileContent: []byte{'c', 'a', 'f', 0xe9},
			want:        true,
		},
		{
			name:        "UTF-16 text",
			fileContent: []byte{0xff, 0xfe, 'h', 0x00, 'i', 0x00},
			want:        true,
		},
		{
			name:        "JPEG",
			fileContent: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'},
			want:        true,
		},
		{
			name:        "PNG",
			fileContent: []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsBinaryContents(tt.fileContent))
		})
	}
}

// Contents that IsBinaryContents reports as text must survive a JSON round
// trip unchanged, since that is how they reach the Gists API.
func TestIsBinaryContentsMatchesJSONRoundTrip(t *testing.T) {
	contents := [][]byte{
		[]byte("package main"),
		[]byte("café 👋 日本語"),
		[]byte("hello\x01world"),
		allASCIIControlChars(),
		{'c', 'a', 'f', 0xe9},
		{0xff, 0xfe, 'h', 0x00, 'i', 0x00},
		{0xff, 0xd8, 0xff, 0xe0},
		{0x89, 'P', 'N', 'G'},
	}

	for _, c := range contents {
		encoded, err := json.Marshal(string(c))
		require.NoError(t, err)

		var decoded string
		require.NoError(t, json.Unmarshal(encoded, &decoded))

		lossless := decoded == string(c)
		assert.Equal(t, lossless, !IsBinaryContents(c),
			"contents %q: survives JSON round trip = %v, treated as text = %v",
			c, lossless, !IsBinaryContents(c))
	}
}

func TestIsBinaryFile(t *testing.T) {
	tests := []struct {
		name        string
		fileContent []byte
		want        bool
	}{
		{
			name:        "text file",
			fileContent: []byte("package main"),
			want:        false,
		},
		{
			// https://github.com/cli/cli/issues/9761
			name:        "text file containing control characters",
			fileContent: []byte("hello\x01world\n"),
			want:        false,
		},
		{
			name:        "JPEG file",
			fileContent: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'},
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "gistfile.txt")
			require.NoError(t, os.WriteFile(file, tt.fileContent, 0600))

			got, err := IsBinaryFile(file)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsBinaryFileMissing(t *testing.T) {
	_, err := IsBinaryFile(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

// allASCIIControlChars returns the 33 ASCII control characters, surrounded by
// ordinary text.
func allASCIIControlChars() []byte {
	contents := []byte("start")
	for i := 0x00; i <= 0x1f; i++ {
		contents = append(contents, byte(i))
	}
	contents = append(contents, 0x7f)
	return append(contents, []byte("end")...)
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
			result, err := GetRawGistFile(client, safeurl.NewImmutableSafeURL("https://gist.githubusercontent.com/raw-url"))

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result.Raw())
			}

			reg.Verify(t)
		})
	}
}
