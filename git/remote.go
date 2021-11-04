package git

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/internal/run"
)

var remoteRE = regexp.MustCompile(`(.+)\s+(.+)\s+\((push|fetch)\)`)

// RemoteSet is a slice of git remotes
type RemoteSet []*Remote

func NewRemote(name string, u string) *Remote {
	pu, _ := url.Parse(u)
	return &Remote{
		Name:     name,
		FetchURL: pu,
		PushURL:  pu,
	}
}

// Remote is a parsed git remote
type Remote struct {
	Name     string
	Resolved string
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
	remotes := parseRemotes(list)

	// this is affected by SetRemoteResolution
	remoteCmd, err := GitCommand("config", "--get-regexp", `^remote\..*\.gh-resolved$`)
	if err != nil {
		return nil, err
	}
	output, _ := run.PrepareCmd(remoteCmd).Output()
	for _, l := range outputLines(output) {
		parts := strings.SplitN(l, " ", 2)
		if len(parts) < 2 {
			continue
		}
		rp := strings.SplitN(parts[0], ".", 3)
		if len(rp) < 2 {
			continue
		}
		name := rp[1]
		for _, r := range remotes {
			if r.Name == name {
				r.Resolved = parts[1]
				break
			}
		}
	}

	return remotes, nil
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

// AddRemote adds a new git remote and auto-fetches objects from it
func AddRemote(name, u string) (*Remote, error) {
	addCmd, err := GitCommand("remote", "add", "-f", name, u)
	if err != nil {
		return nil, err
	}
	err = run.PrepareCmd(addCmd).Run()
	if err != nil {
		return nil, err
	}

	var urlParsed *url.URL
	if strings.HasPrefix(u, "https") {
		urlParsed, err = url.Parse(u)
		if err != nil {
			return nil, err
		}

	} else {
		urlParsed, err = ParseURL(u)
		if err != nil {
			return nil, err
		}

	}

	return &Remote{
		Name:     name,
		FetchURL: urlParsed,
		PushURL:  urlParsed,
	}, nil
}

func UpdateRemoteURL(name, u string) error {
	addCmd, err := GitCommand("remote", "set-url", name, u)
	if err != nil {
		return err
	}
	return run.PrepareCmd(addCmd).Run()
}

func SetRemoteResolution(name, resolution string) error {
	addCmd, err := GitCommand("config", "--add", fmt.Sprintf("remote.%s.gh-resolved", name), resolution)
	if err != nil {
		return err
	}
	return run.PrepareCmd(addCmd).Run()
}
