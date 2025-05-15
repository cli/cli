package shared

import (
	"context"

	"github.com/cli/cli/v2/git"
)

var _ GitConfigClient = &CachedBranchConfigGitConfigClient{}

type CachedBranchConfigGitConfigClient struct {
	CachedBranchConfig git.BranchConfig
	GitConfigClient
}

func (c CachedBranchConfigGitConfigClient) ReadBranchConfig(ctx context.Context, branchName string) (git.BranchConfig, error) {
	return c.CachedBranchConfig, nil
}
