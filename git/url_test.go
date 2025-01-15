package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "scp-like",
			url:  "git@example.com:owner/repo",
			want: true,
		},
		{
			name: "scp-like with no user",
			url:  "example.com:owner/repo",
			want: false,
		},
		{
			name: "ssh",
			url:  "ssh://git@example.com/owner/repo",
			want: true,
		},
		{
			name: "git",
			url:  "git://example.com/owner/repo",
			want: true,
		},
		{
			name: "git with extension",
			url:  "git://example.com/owner/repo.git",
			want: true,
		},
		{
			name: "git+ssh",
			url:  "git+ssh://git@example.com/owner/repo.git",
			want: true,
		},
		{
			name: "https",
			url:  "https://example.com/owner/repo.git",
			want: true,
		},
		{
			name: "git+https",
			url:  "git+https://example.com/owner/repo.git",
			want: true,
		},
		{
			name: "no protocol",
			url:  "example.com/owner/repo",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsURL(tt.url))
		})
	}
}

func TestParseURL(t *testing.T) {
	type url struct {
		Scheme string
		User   string
		Host   string
		Path   string
	}
	tests := []struct {
		name    string
		url     string
		want    url
		wantErr bool
	}{
		{
			name: "HTTPS",
			url:  "https://example.com/owner/repo.git",
			want: url{
				Scheme: "https",
				User:   "",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "HTTP",
			url:  "http://example.com/owner/repo.git",
			want: url{
				Scheme: "http",
				User:   "",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "git",
			url:  "git://example.com/owner/repo.git",
			want: url{
				Scheme: "git",
				User:   "",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "ssh",
			url:  "ssh://git@example.com/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "ssh with port",
			url:  "ssh://git@example.com:443/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "ssh, ipv6",
			url:  "ssh://git@[::1]/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "[::1]",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "ssh with port, ipv6",
			url:  "ssh://git@[::1]:22/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "[::1]",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "git+ssh",
			url:  "git+ssh://example.com/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "git+https",
			url:  "git+https://example.com/owner/repo.git",
			want: url{
				Scheme: "https",
				User:   "",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "scp-like",
			url:  "git@example.com:owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "scp-like, leading slash",
			url:  "git@example.com:/owner/repo.git",
			want: url{
				Scheme: "ssh",
				User:   "git",
				Host:   "example.com",
				Path:   "/owner/repo.git",
			},
		},
		{
			name: "file protocol",
			url:  "file:///example.com/owner/repo.git",
			want: url{
				Scheme: "file",
				User:   "",
				Host:   "",
				Path:   "/example.com/owner/repo.git",
			},
		},
		{
			name: "file path",
			url:  "/example.com/owner/repo.git",
			want: url{
				Scheme: "",
				User:   "",
				Host:   "",
				Path:   "/example.com/owner/repo.git",
			},
		},
		{
			name: "Windows file path",
			url:  "C:\\example.com\\owner\\repo.git",
			want: url{
				Scheme: "c",
				User:   "",
				Host:   "",
				Path:   "",
			},
		},
		{
			name:    "fails to parse",
			url:     "ssh://git@[/tmp/git-repo",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := ParseURL(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			assert.Equal(t, u.Scheme, tt.want.Scheme)
			assert.Equal(t, u.User.Username(), tt.want.User)
			assert.Equal(t, u.Host, tt.want.Host)
			assert.Equal(t, u.Path, tt.want.Path)
		})
	}
}
