package github

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/github/gh-cli/git"
)

var (
	OriginNamesInLookupOrder = []string{"upstream", "github", "origin"}
)

type Remote struct {
	Name    string
	URL     *url.URL
	PushURL *url.URL
}

func (remote *Remote) String() string {
	return remote.Name
}

func (remote *Remote) Project() (*Project, error) {
	p, err := NewProjectFromURL(remote.URL)
	if _, ok := err.(*GithubHostError); ok {
		return NewProjectFromURL(remote.PushURL)
	}
	return p, err
}

func Remotes() (remotes []Remote, err error) {
	re := regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

	rs, err := git.Remotes()
	if err != nil {
		err = fmt.Errorf("Can't load git remote")
		return
	}

	// build the remotes map
	remotesMap := make(map[string]map[string]string)
	for _, r := range rs {
		if re.MatchString(r) {
			match := re.FindStringSubmatch(r)
			name := strings.TrimSpace(match[1])
			url := strings.TrimSpace(match[2])
			urlType := strings.TrimSpace(match[3])
			utm, ok := remotesMap[name]
			if !ok {
				utm = make(map[string]string)
				remotesMap[name] = utm
			}
			utm[urlType] = url
		}
	}

	// construct remotes in priority order
	names := OriginNamesInLookupOrder
	for _, name := range names {
		if u, ok := remotesMap[name]; ok {
			r, err := newRemote(name, u)
			if err == nil {
				remotes = append(remotes, r)
				delete(remotesMap, name)
			}
		}
	}

	// the rest of the remotes
	for n, u := range remotesMap {
		r, err := newRemote(n, u)
		if err == nil {
			remotes = append(remotes, r)
		}
	}

	return
}

func newRemote(name string, urlMap map[string]string) (Remote, error) {
	r := Remote{}

	fetchURL, ferr := git.ParseURL(urlMap["fetch"])
	pushURL, perr := git.ParseURL(urlMap["push"])
	if ferr != nil && perr != nil {
		return r, fmt.Errorf("No valid remote URLs")
	}

	r.Name = name
	if ferr == nil {
		r.URL = fetchURL
	}
	if perr == nil {
		r.PushURL = pushURL
	}

	return r, nil
}

// GuessRemote attempts to select and return the remote a user likely wants to target when dealing with GitHub repositories.
func GuessRemote() (Remote, error) {

	remotes, err := Remotes()
	if err != nil {
		return Remote{}, err
	}

	if len(remotes) == 0 {
		return Remote{}, fmt.Errorf("unable to guess remote")
	}

	// lol
	return remotes[0], nil
}
