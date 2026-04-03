package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/cli/cli/v2/internal/ghrepo"
)

var discussionURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/discussions/(\d+)`)

// ParseDiscussionArg parses a discussion number or URL from a command argument.
// It returns the discussion number and, if the argument was a URL, a repo override.
func ParseDiscussionArg(arg string) (int, ghrepo.Interface, error) {
	if num, err := strconv.Atoi(arg); err == nil {
		return num, nil, nil
	}

	if len(arg) > 1 && arg[0] == '#' {
		if num, err := strconv.Atoi(arg[1:]); err == nil {
			return num, nil, nil
		}
	}

	u, err := url.Parse(arg)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, nil, fmt.Errorf("invalid discussion argument: %s", arg)
	}

	m := discussionURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return 0, nil, fmt.Errorf("invalid discussion URL: %s", arg)
	}

	num, _ := strconv.Atoi(m[3])
	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	return num, repo, nil
}
