package shared

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
)

// PRFromArgs looks up the pull request from either the number/branch/URL argument or one belonging to the current branch
//
// NOTE: this API isn't great, but is here as a compatibility layer between old-style and new-style commands
func PRFromArgs(apiClient *api.Client, baseRepoFn func() (ghrepo.Interface, error), branchFn func() (string, error), remotesFn func() (context.Remotes, error), arg string) (*api.PullRequest, ghrepo.Interface, error) {
	if arg != "" {
		// First check to see if the prString is a url, return repo from url if found. This
		// is run first because we don't need to run determineBaseRepo for this path
		pr, r, err := prFromURL(apiClient, arg)
		if pr != nil || err != nil {
			return pr, r, err
		}
	}

	repo, err := baseRepoFn()
	if err != nil {
		return nil, nil, fmt.Errorf("could not determine base repo: %w", err)
	}

	// If there are no args see if we can guess the PR from the current branch
	if arg == "" {
		pr, err := prForCurrentBranch(apiClient, repo, branchFn, remotesFn)
		return pr, repo, err
	} else {
		// Next see if the prString is a number and use that to look up the url
		pr, err := prFromNumberString(apiClient, repo, arg)
		if pr != nil || err != nil {
			return pr, repo, err
		}

		// Last see if it is a branch name
		pr, err = api.PullRequestForBranch(apiClient, repo, "", arg)
		return pr, repo, err
	}
}

func prFromNumberString(apiClient *api.Client, repo ghrepo.Interface, s string) (*api.PullRequest, error) {
	if prNumber, err := strconv.Atoi(strings.TrimPrefix(s, "#")); err == nil {
		return api.PullRequestByNumber(apiClient, repo, prNumber)
	}

	return nil, nil
}

var pullURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/pull/(\d+)`)

func prFromURL(apiClient *api.Client, s string) (*api.PullRequest, ghrepo.Interface, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, nil, nil
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, nil, nil
	}

	m := pullURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, nil, nil
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	prNumberString := m[3]
	pr, err := prFromNumberString(apiClient, repo, prNumberString)
	return pr, repo, err
}

func prForCurrentBranch(apiClient *api.Client, repo ghrepo.Interface, branchFn func() (string, error), remotesFn func() (context.Remotes, error)) (*api.PullRequest, error) {
	prHeadRef, err := branchFn()
	if err != nil {
		return nil, err
	}

	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		return prFromNumberString(apiClient, repo, m[1])
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		rem, _ := remotesFn()
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			prHeadRef = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, repo.RepoOwner()) {
			prHeadRef = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	return api.PullRequestForBranch(apiClient, repo, "", prHeadRef)
}
