package source

import (
	"fmt"
	"strings"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"

	"github.com/cli/cli/v2/internal/ghrepo"
)

const SupportedHost = "github.com"

// BuildRepoURL returns the canonical repository URL stored in skill metadata.
func BuildRepoURL(host, owner, repo string) string {
	return ghrepo.GenerateRepoURL(ghrepo.NewWithHost(owner, repo, host), "")
}

// ParseRepoURL parses a repository URL stored in skill metadata.
func ParseRepoURL(raw string) (ghrepo.Interface, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("repository URL is empty")
	}

	repo, err := ghrepo.FromFullName(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid repository URL %q: %w", raw, err)
	}

	return repo, nil
}

// ParseMetadataRepo extracts repository information from skill metadata.
func ParseMetadataRepo(meta map[string]interface{}) (ghrepo.Interface, bool, error) {
	if meta == nil {
		return nil, false, nil
	}

	repoValue, _ := meta["github-repo"].(string)
	if repoValue == "" {
		return nil, false, nil
	}

	repo, err := ParseRepoURL(repoValue)
	if err != nil {
		return nil, true, err
	}

	return repo, true, nil
}

// ValidateSupportedHost rejects hosts that are not supported.
// Supported hosts are github.com and GHEC with data residency (*.ghe.com).
// GitHub Enterprise Server is not currently supported.
func ValidateSupportedHost(host string) error {
	host = normalizeHost(host)
	if host == "" {
		return fmt.Errorf("could not determine repository host")
	}
	if host == SupportedHost || ghauth.IsTenancy(host) {
		return nil
	}
	if ghauth.IsEnterprise(host) {
		return fmt.Errorf("GitHub Skills does not currently support GitHub Enterprise Server; got %s", host)
	}
	return fmt.Errorf("unsupported host for GitHub Skills: %s", host)
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	return strings.TrimPrefix(host, "www.")
}
