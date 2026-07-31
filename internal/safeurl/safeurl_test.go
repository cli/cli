package safeurl_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/stretchr/testify/require"
)

var _ safeurl.SafeURL = (*safeurl.MutableSafeURL)(nil)
var _ safeurl.SafeURL = (*safeurl.ImmutableSafeURL)(nil)

func TestRepoPartsFromNWO(t *testing.T) {

	tests := []struct {
		name      string
		nwo       string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{
			name:      "owner and repo",
			nwo:       "octocat/hello-world",
			wantOwner: "octocat",
			wantName:  "hello-world",
		},
		{
			name:    "no separator",
			nwo:     "octocat",
			wantErr: true,
		},
		{
			name:    "empty",
			nwo:     "",
			wantErr: true,
		},
		{
			name:    "missing name",
			nwo:     "octocat/",
			wantErr: true,
		},
		{
			name:    "missing owner",
			nwo:     "/hello-world",
			wantErr: true,
		},
		{
			name:      "parts are returned unescaped",
			nwo:       "my owner/my repo",
			wantOwner: "my owner",
			wantName:  "my repo",
		},
		{
			name:    "extra separators are rejected",
			nwo:     "foo/bar/codespaces",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			owner, name, err := safeurl.RepoPartsFromNWO(tt.nwo)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantOwner, owner)
				require.Equal(t, tt.wantName, name)
			}
		})
	}
}

func TestJoinPathRejectsTraversal(t *testing.T) {
	tests := []struct {
		name       string
		components []string
	}{
		{
			name:       "only a .. component",
			components: []string{".."},
		},
		{
			name:       "a .. component in the middle",
			components: []string{"repos", "octocat", "..", "hello-world"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errJoinPath := safeurl.JoinPath(tt.components...)
			require.Error(t, errJoinPath)
			_, errJoinPathWithHostPrefix := safeurl.JoinPathWithHostPrefix("https://api.github.com", tt.components...)
			require.Error(t, errJoinPathWithHostPrefix)
		})
	}
}

func TestMutableSafeURLString(t *testing.T) {
	tests := []struct {
		name string
		url  func(t *testing.T) (*safeurl.MutableSafeURL, error)
		want string
	}{
		{
			name: "zero value renders empty",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return &safeurl.MutableSafeURL{}, nil
			},
			want: "",
		},
		{
			name: "path only",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("foo", "bar", "baz")
			},
			want: "foo/bar/baz",
		},
		{
			name: "single path component",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("foo")
			},
			want: "foo",
		},
		{
			name: "empty components produce empty segments",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("", "bar", "")
			},
			want: "/bar/",
		},
		{
			name: "escapes path components",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("foo", "bar baz", "a/b")
			},
			want: "foo/bar%20baz/a%2Fb",
		},
		{
			name: "pre-encoded dot-dot cannot bypass the traversal check",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("foo", "bar", "%2e%2e", "baz")
			},
			want: "foo/bar/%252e%252e/baz",
		},
		{
			name: "single dot component is preserved verbatim",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath("foo", "bar", ".", "baz")
			},
			want: "foo/bar/./baz",
		},
		{
			name: "leading single dot components are preserved verbatim",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPath(".", ".", "foo", "bar")
			},
			want: "././foo/bar",
		},
		{
			name: "pre-encoded dot-dot cannot bypass the traversal check with host prefix",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host", "foo", "bar", "%2e%2e", "baz")
			},
			want: "https://host/foo/bar/%252e%252e/baz",
		},
		{
			name: "single dot component is preserved verbatim with host prefix",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host", "foo", "bar", ".", "baz")
			},
			want: "https://host/foo/bar/./baz",
		},
		{
			name: "leading single dot components are preserved verbatim with host prefix",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host", ".", ".", "foo", "bar")
			},
			want: "https://host/././foo/bar",
		},
		{
			name: "host prefix and path",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host", "foo", "bar", "baz")
			},
			want: "https://host/foo/bar/baz",
		},
		{
			name: "host prefix remains intact",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host/with/slash", "foo", "bar", "baz")
			},
			want: "https://host/with/slash/foo/bar/baz",
		},
		{
			name: "host prefix with trailing slash",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host/", "foo", "bar", "baz")
			},
			want: "https://host/foo/bar/baz",
		},
		{
			name: "host prefix without path",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host")
			},
			want: "https://host",
		},
		{
			name: "host prefix with trailing slash and no path",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				return safeurl.JoinPathWithHostPrefix("https://host/")
			},
			want: "https://host/",
		},
		{
			name: "query only",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				u := &safeurl.MutableSafeURL{}
				u.SetQuery("page", "2")
				return u, nil
			},
			want: "?page=2",
		},
		{
			name: "path and query",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				u, err := safeurl.JoinPath("foo", "bar", "baz")
				require.NoError(t, err)
				u.SetQuery("value", "x")
				return u, nil
			},
			want: "foo/bar/baz?value=x",
		},
		{
			name: "host prefix, path, and query",
			url: func(t *testing.T) (*safeurl.MutableSafeURL, error) {
				u, err := safeurl.JoinPathWithHostPrefix("https://host", "foo", "bar")
				require.NoError(t, err)
				u.SetQuery("value", "x y")
				return u, nil
			},
			want: "https://host/foo/bar?value=x+y",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := tt.url(t)
			require.NoError(t, err)
			require.Equal(t, tt.want, u.String())
		})
	}
}

func TestMutableSafeURLSetQuery(t *testing.T) {
	type query struct {
		key   string
		value string
	}

	tests := []struct {
		name    string
		queries []query
		want    string
	}{
		{
			name:    "replaces existing value rather than appending",
			queries: []query{{"a", "1"}, {"a", "2"}},
			want:    "foo/bar?a=2",
		},
		{
			name:    "sorts keys deterministically",
			queries: []query{{"b", "2"}, {"a", "1"}},
			want:    "foo/bar?a=1&b=2",
		},
		{
			name:    "escapes keys and values",
			queries: []query{{"a", "x y&z"}},
			want:    "foo/bar?a=x+y%26z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := safeurl.JoinPath("foo", "bar")
			require.NoError(t, err)
			for _, q := range tt.queries {
				u.SetQuery(q.key, q.value)
			}
			require.Equal(t, tt.want, u.String())
		})
	}
}

func TestImmutableSafeURLString(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "empty renders empty",
			url:  "",
			want: "",
		},
		{
			name: "renders the wrapped url verbatim without encoding",
			url:  "https://host/foo/bar baz/?value=x y",
			want: "https://host/foo/bar baz/?value=x y",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, safeurl.NewImmutableSafeURL(tt.url).String())
		})
	}
}
