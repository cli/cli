package shared

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/prompt"
	"gopkg.in/yaml.v3"
)

// TODO scope stats by repo
// TODO generalize this
// TODO scope stats by data type
// TODO should work with:
// - issues
// - prs
// - gists
// - repos
// - (?) releases
// - (?) runs
// - (?) workflows
// - (?) ssh key
// - (?) extensions

type ByLastAccess []issueWithStats

func (l ByLastAccess) Len() int {
	return len(l)
}
func (l ByLastAccess) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
func (l ByLastAccess) Less(i, j int) bool {
	return l[i].Last.After(l[j].Last)
}

type ByFrecency []issueWithStats

func (f ByFrecency) Len() int {
	return len(f)
}
func (f ByFrecency) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}
func (f ByFrecency) Less(i, j int) bool {
	iScore := f[i].CountEntry.Score()
	jScore := f[j].CountEntry.Score()
	if iScore == jScore {
		return f[i].Last.After(f[j].Last)
	}
	return iScore > jScore
}

type issueWithStats struct {
	api.Issue
	CountEntry
}

func sortByFrecent(issues []api.Issue, frecent map[int]*CountEntry) []string {
	withStats := []issueWithStats{}
	for _, i := range issues {
		entry, ok := frecent[i.Number]
		if !ok {
			entry = &CountEntry{}
		}
		withStats = append(withStats, issueWithStats{
			Issue:      i,
			CountEntry: *entry,
		})
	}
	sort.Sort(ByLastAccess(withStats))
	previousIssue := withStats[0]
	withStats = withStats[1:]
	sort.Stable(ByFrecency(withStats))
	choices := []string{fmt.Sprintf("%d", previousIssue.Number)}
	for _, ws := range withStats {
		choices = append(choices, fmt.Sprintf("%d", ws.Number))
	}
	return choices
}

func SelectFrecent(c *http.Client, repo ghrepo.Interface) (string, error) {
	client := api.NewCachedClient(c, time.Hour*6)

	issues, err := getIssues(client, repo)
	if err != nil {
		return "", err
	}

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
	Last  time.Time
	Count int
}

func (c CountEntry) Score() int {
	if c.Count == 0 {
		return 0
	}
	duration := time.Since(c.Last)
	recencyScore := 10
	if duration < 1*time.Hour {
		recencyScore = 100
	} else if duration < 6*time.Hour {
		recencyScore = 80
	} else if duration < 24*time.Hour {
		recencyScore = 60
	} else if duration < 3*24*time.Hour {
		recencyScore = 40
	} else if duration < 7*24*time.Hour {
		recencyScore = 20
	}

	return c.Count * recencyScore
}

type FrecentEntry struct {
	Issues map[int]*CountEntry
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

	return &stateEntry, nil
}

func UpdateFrecent(issueNumber int) error {
	frecentPath := defaultFrecentPath()
	frecent, err := getFrecentEntry(frecentPath)
	if err != nil {
		return err
	}
	count, ok := frecent.Issues[issueNumber]
	if !ok {
		count = &CountEntry{}
		frecent.Issues[issueNumber] = count
	}
	count.Count++
	count.Last = time.Now()
	content, err := yaml.Marshal(frecent)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(frecentPath), 0755)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(frecentPath, content, 0600)
}

func getIssues(c *http.Client, repo ghrepo.Interface) ([]api.Issue, error) {
	apiClient := api.NewClientFromHTTP(c)
	query := `query GetIssueNumbers($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issues(first:100, orderBy: {field: UPDATED_AT, direction: DESC}, states: [OPEN]) {
			    nodes {
					 number
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
