package frecency

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
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

/** Manager:
 	- create the frecency.yml (constructor)
	- manage the different data types (issues, prs, repos)
		- identifiers ?

  	- update the frecency.yaml file // on different commands like `gh repo view` and so on...
		- mutex (lock) on the file when updating

	- garbage collecting the frecency file
		- remove closed issues, prs, deleted repos, etc.
		- maybe only keep a # of issues, prs, and repos stored in the yaml file. (20?)
		- will be fairly expensive, how to schedule?

- modifying the gh __ view commands to show suggestions
- in pkg/cmd/frecency  --- add some configuration commands?
	- `gh frecency clear` etc.

Issues to consider:
- privacy risk of frecent.yml
- renamed repo
- updating yml risky, makes gh stateful

/*

-- make more agnostic to types, (more like plain key-value store ?)

cli/cli:
	last: 2021-11-17T16:26:48.6695015-06:00
	count: 20 # when you use `gh repo view`
	issues:
		6667: # when you use `gh issue view`
			last: 2021-11-17T16:26:48.6695015-06:00
			count: 15
		4567:
			last: 2021-11-17T16:26:48.6695015-06:00
			count: 1
	prs: # when you use `gh pr view`
		1234:
			last: 2021-11-17T16:26:48.6695015-06:00
			count: 12

cli/go-gh:
	# same for other repos

*/

// GetFrecent, RecordAccess
type Manager struct {
	config config.Config
	io     *iostreams.IOStreams
}

func NewManager(io *iostreams.IOStreams) *Manager {
	return &Manager{
		io: io,
	}
}

// create the frecency.yml
func (m *Manager) initFrecentFile() error {
	_, err := os.Create("frecent.yml")
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) SetConfig(cfg config.Config) {
	m.config = cfg
}

func (m *Manager) FrecentPath() string {
	return filepath.Join(config.StateDir(), "frecent.yml")
}

type FrecentEntry map[string]*RepoEntry

//TODO: repo is identified by full name
// will get messed up if renamed
type Identifier struct {
	Repository string
	Number     int
}

type RepoEntry struct {
	Issues map[int]*CountEntry `yaml: "issues"`
	PRs    map[int]*CountEntry `yaml: "prs"`
}

type CountEntry struct {
	Last  time.Time
	Count int
}

func (m *Manager) RecordAccess(dataType string, id Identifier) error {
	frecentPath := m.FrecentPath()
	frecent, err := m.getFrecentEntry(frecentPath)
	if err != nil {
		return err
	}

	var count *CountEntry
	repoEntry, ok := (*frecent)[id.Repository]
	if !ok {
		(*frecent)[id.Repository] = &RepoEntry{}
		repoEntry = (*frecent)[id.Repository]
	}
	if dataType == "issue" {
		count, ok := repoEntry.Issues[id.Number]
		if !ok {
			count = &CountEntry{}
			repoEntry.Issues[id.Number] = count
		}
	} else if dataType == "pr" {
		count, ok := repoEntry.PRs[id.Number]
		if !ok {
			count = &CountEntry{}
			repoEntry.PRs[id.Number] = count
		}
	} else {
		return fmt.Errorf("Not implemented")
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

func (m *Manager) getFrecentEntry(frecentPath string) (*FrecentEntry, error) {
	content, err := ioutil.ReadFile(frecentPath)
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

// sorting
type issueWithStats struct {
	api.Issue
	CountEntry
}

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
