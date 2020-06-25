package newlygit

import (
	"context"
	"math/rand"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

func shuffleOptions(all []string, answer string, length int) []string {
	x := []string{}

	// Remove answer from list
	for _, i := range all {
		if i != answer {
			x = append(x, i)
		}
	}
	rand.Shuffle(len(x), func(i, j int) {
		x[i], x[j] = x[j], x[i]
	})
	x = x[:length]

	randomIndex := rand.Intn(len(x))
	return append(x[:randomIndex], append([]string{answer}, x[randomIndex:]...)...)
}

func allContributors(client *http.Client, baseRepo ghrepo.Interface) ([]string, error) {
	var query struct {
		Repository struct {
			Collaborators struct {
				Nodes []struct {
					Login string
				}
			} `graphql:"collaborators(last: 100)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(baseRepo.RepoOwner()),
		"name":  githubv4.String(baseRepo.RepoName()),
	}

	v4 := githubv4.NewClient(client)
	contributors := []string{}
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return nil, err
	}
	for _, node := range query.Repository.Collaborators.Nodes {
		contributors = append(contributors, node.Login)
	}

	return contributors, nil
}

func randomPullRequest(client *http.Client, baseRepo ghrepo.Interface) (*api.PullRequest, error) {
	numbers, err := allPullRequestNumbers(client, baseRepo)
	if err != nil {
		return nil, err
	}
	i := rand.Intn(len(numbers))
	number := numbers[i]
	c := api.ClientFromHttpClient(client)
	return api.PullRequestByNumber(c, baseRepo, number)
}

func allPullRequestNumbers(client *http.Client, baseRepo ghrepo.Interface) ([]int, error) {
	var query struct {
		Repository struct {
			PullRequests struct {
				TotalCount int
				Nodes      []struct {
					Number int
				}
			} `graphql:"pullRequests(last: 100)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(baseRepo.RepoOwner()),
		"name":  githubv4.String(baseRepo.RepoName()),
	}

	v4 := githubv4.NewClient(client)
	numbers := []int{}
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return nil, err
	}
	for _, node := range query.Repository.PullRequests.Nodes {
		numbers = append(numbers, node.Number)
	}

	return numbers, nil
}
