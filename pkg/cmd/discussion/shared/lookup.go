package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
)

var discussionURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/discussions/(\d+)$`)

// ParseDiscussionArg parses a discussion number or URL from a command argument.
// It returns the discussion number and, if the argument was a URL, a repo override.
func ParseDiscussionArg(arg string) (int32, ghrepo.Interface, error) {
	if num, err := strconv.ParseInt(arg, 10, 32); err == nil {
		return int32(num), nil, nil
	}

	if len(arg) > 1 && arg[0] == '#' {
		if num, err := strconv.ParseInt(arg[1:], 10, 32); err == nil {
			return int32(num), nil, nil
		}
	}

	u, err := url.Parse(arg)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, nil, fmt.Errorf("invalid discussion argument: %q", arg)
	}

	// An HTTP URL is also accepted because we only extract the discussion number,
	// repo and host from the URL path; no API calls are made over HTTP.

	m := discussionURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return 0, nil, fmt.Errorf("invalid discussion URL: %q", arg)
	}

	num, err := strconv.ParseInt(m[3], 10, 32)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid discussion number in URL: %q", m[3])
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	return int32(num), repo, nil
}

// ParsedDiscussionOrCommentArg holds the result of parsing a comment command argument.
// Depending on the input, different fields are populated:
//   - Discussion number (e.g., "123") or URL (e.g., "https://github.com/OWNER/REPO/discussions/123"):
//     Number and optionally Repo are set.
//   - Comment URL (e.g., "https://github.com/OWNER/REPO/discussions/123#discussioncomment-456"):
//     Number, Repo, and CommentDatabaseID are set.
//   - Comment node ID (e.g., "DC_kwDOOokwWs4BBmcq"):
//     only CommentNodeID is set.
type ParsedDiscussionOrCommentArg struct {
	Number            int32
	Repo              ghrepo.Interface
	CommentDatabaseID int64
	CommentNodeID     string
}

// ParseDiscussionOrCommentArg parses a positional argument that can be a discussion number,
// discussion URL, comment node ID (DC_...), or comment URL (with a "#discussioncomment-NNNNN" fragment).
func ParseDiscussionOrCommentArg(arg string) (*ParsedDiscussionOrCommentArg, error) {
	if strings.HasPrefix(arg, "DC_") {
		return &ParsedDiscussionOrCommentArg{CommentNodeID: arg}, nil
	}

	if num, err := strconv.ParseInt(arg, 10, 32); err == nil {
		return &ParsedDiscussionOrCommentArg{Number: int32(num)}, nil
	}
	if len(arg) > 1 && arg[0] == '#' {
		if num, err := strconv.ParseInt(arg[1:], 10, 32); err == nil {
			return &ParsedDiscussionOrCommentArg{Number: int32(num)}, nil
		}
	}

	u, err := url.Parse(arg)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid argument: %q (expected a discussion number, URL, or comment ID)", arg)
	}

	m := discussionURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, fmt.Errorf("invalid discussion URL: %q", arg)
	}

	num, err := strconv.ParseInt(m[3], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid discussion number in URL: %q", m[3])
	}
	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())

	if fragment := u.Fragment; strings.HasPrefix(fragment, "discussioncomment-") {
		commentNumStr := strings.TrimPrefix(fragment, "discussioncomment-")
		commentNum, err := strconv.ParseInt(commentNumStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid comment ID in URL fragment: %q", fragment)
		}
		return &ParsedDiscussionOrCommentArg{
			Number:            int32(num),
			Repo:              repo,
			CommentDatabaseID: commentNum,
		}, nil
	}

	return &ParsedDiscussionOrCommentArg{Number: int32(num), Repo: repo}, nil
}
