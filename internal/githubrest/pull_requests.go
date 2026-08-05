package githubrest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// reviewRequest is the request body shared by adding and removing review
// requests. The API rejects null in place of an empty array.
type reviewRequest struct {
	Reviewers     []string `json:"reviewers"`
	TeamReviewers []string `json:"team_reviewers"`
}

// AddPullRequestReviews requests reviews from the given users and teams.
// Team identifiers may be in "org/slug" format.
func AddPullRequestReviews(ctx context.Context, client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	return changeReviewRequests(ctx, client, http.MethodPost, repo, prNumber, users, teams)
}

// RemovePullRequestReviews withdraws review requests from the given users and
// teams. Team identifiers may be in "org/slug" format.
func RemovePullRequestReviews(ctx context.Context, client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	return changeReviewRequests(ctx, client, http.MethodDelete, repo, prNumber, users, teams)
}

func changeReviewRequests(ctx context.Context, client *Client, method string, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}
	if users == nil {
		users = []string{}
	}

	path, err := safeurl.JoinPath(
		"repos",
		repo.RepoOwner(),
		repo.RepoName(),
		"pulls",
		strconv.Itoa(prNumber),
		"requested_reviewers",
	)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(reviewRequest{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}); err != nil {
		return err
	}

	req, err := client.NewRequest(ctx, method, path.String(), buf)
	if err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; no caller needs it.
	_, err = client.Do(req, nil)
	return err
}

// extractTeamSlugs returns just the slug portion of team identifiers, which may
// be given as "org/slug".
func extractTeamSlugs(teams []string) []string {
	slugs := make([]string, 0, len(teams))
	for _, t := range teams {
		if t == "" {
			continue
		}
		s := strings.SplitN(t, "/", 2)
		slugs = append(slugs, s[len(s)-1])
	}
	return slugs
}
