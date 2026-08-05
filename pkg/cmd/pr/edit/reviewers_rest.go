package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

// addPullRequestReviews adds the given user and team reviewers to a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func addPullRequestReviews(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
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
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodPost, path.String(), buf)
	if err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	_, err = client.Do(req, nil)
	return err
}

// removePullRequestReviews removes requested reviewers from a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func removePullRequestReviews(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
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
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), buf)
	if err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	_, err = client.Do(req, nil)
	return err
}

// extractTeamSlugs extracts just the slug portion from team identifiers.
// Team identifiers can be in "org/slug" format; this returns just the slug.
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
