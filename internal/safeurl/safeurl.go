// Package safeurl provides helpers for building REST API URL paths (and full
// URLs, when a host prefix is supplied) from variable components so that user
// or server controlled values cannot break the path or change which resource
// is addressed.
package safeurl

import (
	"fmt"
	"net/url"
	"strings"
)

// RepoPartsFromNWO parses a raw "owner/repo" string and returns the owner and name
// unescaped. It returns an error unless nwo contains exactly one slash with a non-empty
// owner and name, so a value carrying extra slashes cannot smuggle additional path
// segments through as the owner or name.
//
// This intentionally does not reuse ghrepo.FromFullName, which accepts the broader
// "[HOST/]OWNER/REPO" form. The call sites here only ever handle a bare "OWNER/REPO",
// so a stricter parse that rejects an unexpected host component is the safer fit.
func RepoPartsFromNWO(nwo string) (owner, name string, err error) {
	parts := strings.Split(nwo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected the \"OWNER/REPO\" format, got %q", nwo)
	}
	return parts[0], parts[1], nil
}

// SafeURL is the sealed interface implemented by the URL types in this package.
// It exists so that a value known to address a safe REST API URL can be passed
// around and rendered without exposing how it was built.
type SafeURL interface {
	String() string

	// The sealed method keeps the set of implementations closed to this package,
	// so callers outside it cannot forge a value that claims to be safe.
	sealed()
}

// MutableSafeURL is a REST API URL built from a host prefix, path components, and query
// parameters. The path components and query parameters are URL encoded (aka
// percent-encoded) when the URL is rendered so that caller supplied values cannot
// alter the structure of the URL or change which resource it addresses; the host
// prefix is used as given. The zero value renders as the empty string.
type MutableSafeURL struct {
	prefix     string
	components []string
	query      url.Values
}

// JoinPath returns a SafeURL for the path made up of the given components. It
// returns an error if any component is exactly "..", which would traverse the URL
// path and change which resource it addresses.
func JoinPath(components ...string) (*MutableSafeURL, error) {
	if err := checkTraversal(components); err != nil {
		return nil, err
	}
	return &MutableSafeURL{components: components}, nil
}

// JoinPathWithHostPrefix returns a SafeURL for the given host prefix and the path
// made up of the given components. It returns an error if any component is exactly
// "..", which would traverse the URL path and change which resource it addresses.
func JoinPathWithHostPrefix(hostPrefix string, components ...string) (*MutableSafeURL, error) {
	if err := checkTraversal(components); err != nil {
		return nil, err
	}
	return &MutableSafeURL{prefix: hostPrefix, components: components}, nil
}

// checkTraversal returns an error if any component is exactly "..". Such a component
// survives percent-encoding as a real path segment and would traverse the URL path.
// A single "." is left alone because it does not traverse and is a legitimate value
// in some paths.
func checkTraversal(components []string) error {
	for _, c := range components {
		if c == ".." {
			return fmt.Errorf("path component %q would traverse the URL path", c)
		}
	}
	return nil
}

func (u *MutableSafeURL) sealed() {}

// SetQuery sets the query parameter key to value, replacing any existing value.
func (u *MutableSafeURL) SetQuery(key, value string) {
	if u.query == nil {
		u.query = url.Values{}
	}
	u.query.Set(key, value)
}

// String renders the full URL. Path components and query parameters are URL encoded
// (aka percent-encoded) while the host prefix is included as given. The zero value
// renders as the empty string.
func (u *MutableSafeURL) String() string {
	result := joinPathWithHostPrefix(u.prefix, u.components...)
	if len(u.query) > 0 {
		result += "?" + u.query.Encode()
	}
	return result
}

// ImmutableSafeURL is a SafeURL that renders a fixed URL string verbatim. It exists
// so that a URL which was not built from percent-encoded components, such as a full
// URL returned by the server (a pagination "next" link, an asset download URL, and
// the like), can still flow through the SafeURL typed code paths. Because the stored
// value is rendered as given without any encoding, it is only safe to wrap a URL that
// was created from trusted components or received from a trusted source.
type ImmutableSafeURL struct {
	url string
}

// NewImmutableSafeURL returns an ImmutableSafeURL that renders url verbatim. Only pass
// a URL you built yourself from trusted components or received from a trusted source,
// such as a server response; this bypasses all percent-encoding, so passing a value
// that embeds unescaped user or third party input reintroduces the injection risk that
// SafeURL exists to prevent.
func NewImmutableSafeURL(url string) *ImmutableSafeURL {
	return &ImmutableSafeURL{url: url}
}

func (u *ImmutableSafeURL) sealed() {}

// String returns the wrapped URL verbatim.
func (u *ImmutableSafeURL) String() string {
	return u.url
}

// joinPath builds a REST API URL path by percent-encoding each component with
// url.PathEscape and joining them with single slash separators.
//
// With no components, the empty string is returned.
func joinPath(components ...string) string {
	// We build the path by hand rather than with url.JoinPath because url.JoinPath runs path.Clean
	// on the result, which resolves any "." or ".." segments. Percent-encoding does not encode dots,
	// so a component equal to "." or ".." would survive escaping and then be collapsed by the clean,
	// silently changing which resource the path addresses.
	escaped := make([]string, len(components))
	for i, c := range components {
		escaped[i] = url.PathEscape(c)
	}
	return strings.Join(escaped, "/")
}

// joinPathWithHostPrefix builds a full REST API URL by prepending hostPrefix to the path produced by
// JoinPath. A single slash is ensured at the join between hostPrefix and the path so they separate
// cleanly without doubling up. When hostPrefix is empty, the JoinPath result is returned intact, and
// when the joined path is empty, hostPrefix is returned intact. hostPrefix is used verbatim while each
// component is percent-encoded.
func joinPathWithHostPrefix(hostPrefix string, components ...string) string {
	path := joinPath(components...)
	if hostPrefix == "" {
		return path
	}
	if path == "" {
		return hostPrefix
	}
	if !strings.HasSuffix(hostPrefix, "/") {
		return hostPrefix + "/" + path
	}
	return hostPrefix + path
}
