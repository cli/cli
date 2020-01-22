package git

import (
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	"github.com/github/gh-cli/utils"
)

var remoteRE = regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

// RemoteSet is a slice of git remotes
type RemoteSet []*Remote

// Remote is a parsed git remote
type Remote struct {
	Name     string
	FetchURL *url.URL
	PushURL  *url.URL
}

func (r *Remote) String() string {
	return r.Name
}

// Remotes gets the git remotes set for the current repo
func Remotes() (RemoteSet, error) {
	list, err := listRemotes()
	if err != nil {
		return nil, err
	}
	return parseRemotes(list), nil
}

func parseRemotes(gitRemotes []string) (remotes RemoteSet) {
	for _, r := range gitRemotes {
		match := remoteRE.FindStringSubmatch(r)
		if match == nil {
			continue
		}
		name := strings.TrimSpace(match[1])
		urlStr := strings.TrimSpace(match[2])
		urlType := strings.TrimSpace(match[3])

		var rem *Remote
		if len(remotes) > 0 {
			rem = remotes[len(remotes)-1]
			if name != rem.Name {
				rem = nil
			}
		}
		if rem == nil {
			rem = &Remote{Name: name}
			remotes = append(remotes, rem)
		}

		u, err := ParseURL(urlStr)
		if err != nil {
			continue
		}

		switch urlType {
		case "fetch":
			rem.FetchURL = u
		case "push":
			rem.PushURL = u
		}
	}
	return
}

// AddRemote adds a new git remote. The initURL is the remote URL with which the
// automatic fetch is made and finalURL, if non-blank, is set as the remote URL
// after the fetch.
func AddRemote(name, initURL, finalURL string) (*Remote, error) {
	addCmd := exec.Command("git", "remote", "add", "-f", name, initURL)
	err := utils.PrepareCmd(addCmd).Run()
	if err != nil {
		return nil, err
	}

	if finalURL == "" {
		finalURL = initURL
	} else {
		setCmd := exec.Command("git", "remote", "set-url", name, finalURL)
		err := utils.PrepareCmd(setCmd).Run()
		if err != nil {
			return nil, err
		}
	}

	finalURLParsed, err := url.Parse(initURL)
	if err != nil {
		return nil, err
	}

	return &Remote{
		Name:     name,
		FetchURL: finalURLParsed,
		PushURL:  finalURLParsed,
	}, nil
}
