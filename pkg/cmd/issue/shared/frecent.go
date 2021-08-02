package shared

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/prompt"
	"gopkg.in/yaml.v3"
)

func sortByFrecent(issues []api.Issue, frecent map[string]CountEntry) []string {
	return []string{"1234", "4567"}

}

func SelectFrecent(c *http.Client, repo ghrepo.Interface) (string, error) {
	client := api.NewCachedClient(c, time.Hour*6)

	issues, err := getIssues(client, repo)
	frecent, err := getFrecentEntry(defaultFrecentPath())
	if err != nil {
		return "", err
	}

	choices := sortByFrecent(issues, frecent.Issues)

	choice := ""
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "Which issue?",
		Options: choices,
	}, &choice)
	if err != nil {
		return "", err
	}

	return choice, nil
}

type CountEntry struct {
	Last  time.Duration
	Count int
}

type FrecentEntry struct {
	Issues map[string]CountEntry
}

func defaultFrecentPath() string {
	return filepath.Join(config.StateDir(), "frecent.yml")
}

func getFrecentEntry(stateFilePath string) (*FrecentEntry, error) {
	content, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry FrecentEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%#v\n", stateEntry)

	return &stateEntry, nil
}

func updateFrecent(issueNumber string) error {
	// TODO how to properly encode this?
	/*
		  issues:
				  6667:
					  last: 10s
						count: 1
					4567:
						last: 10m
						count: 15
					7890:
						last: 5m
						count: 1
					3456:
						last: 40d
						count: 30
	*/

	// recency values:
	// TODO tweak for shorter access times, people are likely to process a
	// variety of issues in a given day
	// - 4 hours 100
	// - 1 day 80
	// - 3 days 60
	// - 7 days 40
	// - 30 days 20
	// - 90 days 10

	// sort score = frequency * recency_score / max number of timestamps

	// question of seeding. on first run, we have no data. it'd be nice to just
	// fill with N repo issues and then start applying tracking from there.
	// could also do an initial sort based on mentions...but if we're not going
	// to keep pulling that query it seems not worth it.

	return nil
}

/*
func setFrecentEntry(stateFilePath string, t time.Time, r ReleaseInfo) error {
	data := FrecentEntry{Issues: []string{"TODO"}}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(stateFilePath), 0755)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(stateFilePath, content, 0600)
	return err
}

*/

func getIssues(c *http.Client, repo ghrepo.Interface) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `query GetIssueNumbers($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issues(first:100, orderBy: {field: UPDATED_AT, direction: DESC}, states: [OPEN]) {
				nodes {
				  edge {
					  number
					}
				}
			}
		}
  }`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}
	type responseData struct {
		Repository struct {
			Issues struct {
				Nodes []api.Issue
			}
		}
	}
	var resp responseData
	err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Repository.Issues.Nodes, nil
}
