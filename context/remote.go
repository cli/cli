package context

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
)

// Remotes represents a set of git remotes
type Remotes []*Remote

// FindByName returns the first Remote whose name matches the list
func (r Remotes) FindByName(names ...string) (*Remote, error) {
	for _, name := range names {
		for _, rem := range r {
			if rem.Name == name || name == "*" {
				return rem, nil
			}
		}
	}
	return nil, fmt.Errorf("no matching remote found")
}

// FindByRepo returns the first Remote that points to a specific GitHub repository
func (r Remotes) FindByRepo(owner, name string) (*Remote, error) {
	for _, rem := range r {
		if strings.EqualFold(rem.RepoOwner(), owner) && strings.EqualFold(rem.RepoName(), name) {
			return rem, nil
		}
	}
	return nil, fmt.Errorf("no matching remote found; looking for %s/%s", owner, name)
}

// Filter remotes by given hostnames, maintains original order
func (r Remotes) FilterByHosts(hosts []string) Remotes {
	fmt.Println("*** Starting FilterByHosts")
	defer fmt.Println("*** Ending FilterByHosts")

	fmt.Printf("\tHosts: (%d)\n", len(hosts))
	for _, host := range hosts {
		fmt.Println("\t\tHost:", host)
	}

	fmt.Printf("\tRemotes: (%d)\n", len(r))
	for _, rr := range r {
		fmt.Printf("\t\tName: %s\n", rr.Name)
		fmt.Printf("\t\tFetchURL: %s\n", rr.FetchURL)
		fmt.Printf("\t\tPushURL: %s\n", rr.PushURL)
		fmt.Printf("\t\tRepo: %#v\n", rr.Repo)
	}

	filtered := make(Remotes, 0)
	for _, rr := range r {
		for _, host := range hosts {
			fmt.Printf("\t\tComparing %s to %s\n", rr.RepoHost(), host)
			if strings.EqualFold(rr.RepoHost(), host) {
				fmt.Printf("\t\tMatched %s to %s\n", rr.RepoHost(), host)
				filtered = append(filtered, rr)
				break
			}
			fmt.Printf("\t\tNo match %s to %s\n", rr.RepoHost(), host)
		}
	}

	fmt.Printf("\tRetained Remotes: (%d)\n", len(filtered))
	for _, rr := range filtered {
		fmt.Printf("\t\tRemote: %#v\n", rr.Name)
	}
	return filtered
}

func (r Remotes) ResolvedRemote() (*Remote, error) {
	for _, rr := range r {
		if rr.Resolved != "" {
			return rr, nil
		}
	}
	return nil, fmt.Errorf("no resolved remote found")
}

func remoteNameSortScore(name string) int {
	switch strings.ToLower(name) {
	case "upstream":
		return 3
	case "github":
		return 2
	case "origin":
		return 1
	default:
		return 0
	}
}

// https://golang.org/pkg/sort/#Interface
func (r Remotes) Len() int      { return len(r) }
func (r Remotes) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r Remotes) Less(i, j int) bool {
	return remoteNameSortScore(r[i].Name) > remoteNameSortScore(r[j].Name)
}

// Remote represents a git remote mapped to a GitHub repository
type Remote struct {
	*git.Remote
	Repo ghrepo.Interface
}

// RepoName is the name of the GitHub repository
func (r Remote) RepoName() string {
	return r.Repo.RepoName()
}

// RepoOwner is the name of the GitHub account that owns the repo
func (r Remote) RepoOwner() string {
	return r.Repo.RepoOwner()
}

// RepoHost is the GitHub hostname that the remote points to
func (r Remote) RepoHost() string {
	return r.Repo.RepoHost()
}

type Translator interface {
	Translate(*url.URL) *url.URL
}

func TranslateRemotes(gitRemotes git.RemoteSet, translator Translator) (remotes Remotes) {
	fmt.Println("*** Starting TranslateRemotes")
	defer fmt.Println("*** Ending TranslateRemotes")

	for _, r := range gitRemotes {
		fmt.Println("\tTranslating: ", r.Name)
		var repo ghrepo.Interface
		if r.FetchURL != nil {
			fmt.Println("\t\tFetch URL:", r.FetchURL)
			translatedFetchURL := translator.Translate(r.FetchURL)
			fmt.Println("\t\tTranslated Fetch URL:", translatedFetchURL)
			repo, _ = ghrepo.FromURL(translatedFetchURL)
		}
		if r.PushURL != nil && repo == nil {
			fmt.Println("\t\tPush URL:", r.PushURL)
			translatedPushURL := translator.Translate(r.PushURL)
			fmt.Println("\t\tTranslated Push URL:", translatedPushURL)
			repo, _ = ghrepo.FromURL(translatedPushURL)
		}
		if repo == nil {
			continue
		}
		remotes = append(remotes, &Remote{
			Remote: r,
			Repo:   repo,
		})
	}
	return
}
