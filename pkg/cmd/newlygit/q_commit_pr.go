package newlygit

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/shurcooL/githubv4"
)

func commit_pr(client *http.Client, baseRepo ghrepo.Interface) (bool, error) {
	prs, err := randomPullRequests(client, baseRepo, 4)
	if err != nil {
		return false, err
	}

	correctPRIndex := rand.Intn(4)
	correctPR := prs[correctPRIndex]

	message, err := commitMessageFromPR(correctPR.Number, client, baseRepo)
	if err != nil {
		return false, err
	}

	message = strings.TrimSpace(strings.Split(message, "\n")[0])

	options := []string{}
	for _, pr := range prs {
		options = append(options, fmt.Sprintf("\"%s\" by %s", pr.Title, pr.Author.Login))
	}

	questionMessage := fmt.Sprintf("Which PR contained a commit with the message \"%s\"", message)
	var qs = []*survey.Question{
		{
			Name: "q",
			Prompt: &survey.Select{
				Message: questionMessage,
				Options: options,
			},
		},
	}

	answers := struct{ Q int }{}

	err = survey.Ask(qs, &answers)
	if err != nil {
		return false, err
	}

	correct := false
	if answers.Q == correctPRIndex {
		correct = true
		fmt.Printf("%s", utils.Green("\n🎉 CORRECT 🎉\n"))
	} else {
		fmt.Printf("%s", utils.Red("\n😫 WRONG 😫\n"))
	}

	fmt.Printf("The answer is %s\n\n", utils.Green(correctPR.Title))
	fmt.Printf("Check out the pr at %s\n", utils.Bold(correctPR.URL))

	return correct, nil
}

func commitMessageFromPR(prNum int, client *http.Client, baseRepo ghrepo.Interface) (string, error) {
	var query struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					Nodes []struct {
						Commit struct {
							Message string
						}
					}
				} `graphql:"commits(last: 100)"`
			} `graphql:"pullRequest(number: $prNumber)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":    githubv4.String(baseRepo.RepoOwner()),
		"name":     githubv4.String(baseRepo.RepoName()),
		"prNumber": githubv4.Int(prNum),
	}

	v4 := githubv4.NewClient(client)
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return "", err
	}

	return query.Repository.PullRequest.Commits.Nodes[0].Commit.Message, nil
}
