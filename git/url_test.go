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
			// Given a possible Git remote URL
			rawURL := tt.url

			// When it is classified
			got := IsURL(rawURL)

			// Then only supported URL forms are accepted
			assert.Equal(t, tt.want, got, "expected URL classification to match the transport form")
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
		name string
		url  string
		want url
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a valid Git remote URL
			rawURL := tt.url

			// When it is normalized
			u, err := ParseURL(rawURL)

			// Then its canonical URL components are returned
			require.NoError(t, err)
			assert.Equal(t, tt.want.Scheme, u.Scheme)
			assert.Equal(t, tt.want.User, u.User.Username())
			assert.Equal(t, tt.want.Host, u.Host)
			assert.Equal(t, tt.want.Path, u.Path)
		})
	}
}

func TestParseURLReportsMalformedInput(t *testing.T) {
	// Given an SSH URL with a malformed bracketed host
	rawURL := "ssh://git@[/tmp/git-repo"

	// When it is parsed
	u, err := ParseURL(rawURL)

	// Then the malformed URL is rejected
	require.Error(t, err)
	assert.Nil(t, u)
}
