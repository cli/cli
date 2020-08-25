package ghrepo

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
)

func Test_repoFromURL(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		result string
		host   string
		err    error
	}{
		{
			name:   "github.com URL",
			input:  "https://github.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
		{
			name:   "github.com URL with trailing slash",
			input:  "https://github.com/monalisa/octo-cat/",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
		{
			name:   "www.github.com URL",
			input:  "http://www.GITHUB.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
		{
			name:   "too many path components",
			input:  "https://github.com/monalisa/octo-cat/pulls",
			result: "",
			host:   "",
			err:    errors.New("invalid path: /monalisa/octo-cat/pulls"),
		},
		{
			name:   "non-GitHub hostname",
			input:  "https://example.com/one/two",
			result: "one/two",
			host:   "example.com",
			err:    nil,
		},
		{
			name:   "filesystem path",
			input:  "/path/to/file",
			result: "",
			host:   "",
			err:    errors.New("no hostname detected"),
		},
		{
			name:   "filesystem path with scheme",
			input:  "file:///path/to/file",
			result: "",
			host:   "",
			err:    errors.New("no hostname detected"),
		},
		{
			name:   "github.com SSH URL",
			input:  "ssh://github.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
		{
			name:   "github.com HTTPS+SSH URL",
			input:  "https+ssh://github.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
		{
			name:   "github.com git URL",
			input:  "git://github.com/monalisa/octo-cat.git",
			result: "monalisa/octo-cat",
			host:   "github.com",
			err:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.input)
			if err != nil {
				t.Fatalf("got error %q", err)
			}

			repo, err := FromURL(u)
			if err != nil {
				if tt.err == nil {
					t.Fatalf("got error %q", err)
				} else if tt.err.Error() == err.Error() {
					return
				}
				t.Fatalf("got error %q", err)
			}

			got := fmt.Sprintf("%s/%s", repo.RepoOwner(), repo.RepoName())
			if tt.result != got {
				t.Errorf("expected %q, got %q", tt.result, got)
			}
			if tt.host != repo.RepoHost() {
				t.Errorf("expected %q, got %q", tt.host, repo.RepoHost())
			}
		})
	}
}

func TestFromFullName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantName  string
		wantHost  string
		wantErr   error
	}{
		{
			name:      "OWNER/REPO combo",
			input:     "OWNER/REPO",
			wantHost:  "github.com",
			wantOwner: "OWNER",
			wantName:  "REPO",
			wantErr:   nil,
		},
		{
			name:    "too few elements",
			input:   "OWNER",
			wantErr: errors.New(`expected the "[HOST/]OWNER/REPO" format, got "OWNER"`),
		},
		{
			name:    "too many elements",
			input:   "a/b/c/d",
			wantErr: errors.New(`expected the "[HOST/]OWNER/REPO" format, got "a/b/c/d"`),
		},
		{
			name:    "blank value",
			input:   "a/",
			wantErr: errors.New(`expected the "[HOST/]OWNER/REPO" format, got "a/"`),
		},
		{
			name:      "with hostname",
			input:     "example.org/OWNER/REPO",
			wantHost:  "example.org",
			wantOwner: "OWNER",
			wantName:  "REPO",
			wantErr:   nil,
		},
		{
			name:      "full URL",
			input:     "https://example.org/OWNER/REPO.git",
			wantHost:  "example.org",
			wantOwner: "OWNER",
			wantName:  "REPO",
			wantErr:   nil,
		},
		{
			name:      "SSH URL",
			input:     "git@example.org:OWNER/REPO.git",
			wantHost:  "example.org",
			wantOwner: "OWNER",
			wantName:  "REPO",
			wantErr:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := FromFullName(tt.input)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("no error in result, expected %v", tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErr.Error(), err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("got error %v", err)
			}
			if r.RepoHost() != tt.wantHost {
				t.Errorf("expected host %q, got %q", tt.wantHost, r.RepoHost())
			}
			if r.RepoOwner() != tt.wantOwner {
				t.Errorf("expected owner %q, got %q", tt.wantOwner, r.RepoOwner())
			}
			if r.RepoName() != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, r.RepoName())
			}
		})
	}
}
