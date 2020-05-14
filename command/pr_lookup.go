package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
)

func getPr(cmd *cobra.Command, prString string) (*api.PullRequest, error) {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return nil, err
	}

	// This assumes the first arg is the pr number
	if prString != "" {
		pr, err := prFromString(apiClient, baseRepo, prString)
		return pr, err
	}

	pr, err := prFromContext(ctx, baseRepo)
	return pr, err
}

func prFromString(apiClient *api.Client, baseRepo ghrepo.Interface, prString string) (*api.PullRequest, error) {
	if prNumber, repo := prNumberAndRepoFromURL(prString); prNumber != 0 {
		return api.PullRequestByNumber(apiClient, repo, prNumber)
	} else if prNumber, err := strconv.Atoi(strings.TrimPrefix(prString, "#")); err == nil {
		return api.PullRequestByNumber(apiClient, baseRepo, prNumber)
	} else if prNumber, r := prNumberAndRepoFromURL(prString); r != nil {
		return api.PullRequestByNumber(apiClient, r, prNumber)
	}

	return api.PullRequestForBranch(apiClient, baseRepo, "", prString)
}

func prNumberAndRepoFromURL(url string) (int, ghrepo.Interface) {
	if m := prURLRE.FindStringSubmatch(url); m != nil {
		prNumber, err := strconv.Atoi(m[3])
		if err == nil {
			return prNumber, ghrepo.New(m[1], m[2])
		}
	}
	return 0, nil
}

func prFromContext(ctx context.Context, baseRepo ghrepo.Interface) (*api.PullRequest, error) {
	prNumber, branchWithOwner, err := prSelectorForCurrentBranch(ctx, baseRepo)
	if err != nil {
		return nil, err
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	if prNumber != 0 {
		pr, err := api.PullRequestByNumber(apiClient, baseRepo, prNumber)
		return pr, err
	}

	pr, err := api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
	return pr, err
}

func prSelectorForCurrentBranch(ctx context.Context, baseRepo ghrepo.Interface) (prNumber int, prHeadRef string, err error) {
	prHeadRef, err = ctx.Branch()
	if err != nil {
		return
	}
	branchConfig := git.ReadBranchConfig(prHeadRef)

	// the branch is configured to merge a special PR head ref
	prHeadRE := regexp.MustCompile(`^refs/pull/(\d+)/head$`)
	if m := prHeadRE.FindStringSubmatch(branchConfig.MergeRef); m != nil {
		prNumber, _ = strconv.Atoi(m[1])
		return
	}

	var branchOwner string
	if branchConfig.RemoteURL != nil {
		// the branch merges from a remote specified by URL
		if r, err := ghrepo.FromURL(branchConfig.RemoteURL); err == nil {
			branchOwner = r.RepoOwner()
		}
	} else if branchConfig.RemoteName != "" {
		// the branch merges from a remote specified by name
		rem, _ := ctx.Remotes()
		if r, err := rem.FindByName(branchConfig.RemoteName); err == nil {
			branchOwner = r.RepoOwner()
		}
	}

	if branchOwner != "" {
		if strings.HasPrefix(branchConfig.MergeRef, "refs/heads/") {
			prHeadRef = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		}
		// prepend `OWNER:` if this branch is pushed to a fork
		if !strings.EqualFold(branchOwner, baseRepo.RepoOwner()) {
			prHeadRef = fmt.Sprintf("%s:%s", branchOwner, prHeadRef)
		}
	}

	return
}
