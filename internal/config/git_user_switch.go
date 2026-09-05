package config

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
)

// GitClientInterface defines the methods we need from git.Client for testing
type GitClientInterface interface {
	IsLocalGitRepo(ctx context.Context) (bool, error)
	Config(ctx context.Context, name string) (string, error)
}

// MaybeAutomaticUserSwitch checks for a gh.user git config setting and switches the active
// GitHub user if necessary. This should be called before authentication-dependent operations.
func MaybeAutomaticUserSwitch(cfg gh.Config) error {
	return maybeAutomaticUserSwitchWithGitClient(cfg, &git.Client{})
}

// maybeAutomaticUserSwitchWithGitClient is the internal implementation that accepts a git client
// for testing purposes.
func maybeAutomaticUserSwitchWithGitClient(cfg gh.Config, gitClient GitClientInterface) error {
	ctx := context.Background()
	
	// Check if we're in a git repo
	isRepo, err := gitClient.IsLocalGitRepo(ctx)
	if err != nil || !isRepo {
		return nil // Not in a git repo, skip switching
	}

	// Try to read gh.user from git config
	configuredUser, err := gitClient.Config(ctx, "gh.user")
	if err != nil {
		// No gh.user configured or other error, skip switching
		return nil
	}

	// Get current active user from gh config
	authCfg := cfg.Authentication()
	hosts := authCfg.Hosts()
	if len(hosts) == 0 {
		return nil // No authenticated hosts
	}

	// For simplicity, use the first host (usually github.com)
	// In the future this could be made smarter to detect the host from git remotes
	hostname := hosts[0]
	
	currentUser, err := authCfg.ActiveUser(hostname)
	if err != nil {
		return nil // No active user
	}

	// If the configured user is different from the current user, switch
	if configuredUser != currentUser {
		// Check if the configured user exists
		knownUsers := authCfg.UsersForHost(hostname)
		userExists := false
		for _, user := range knownUsers {
			if user == configuredUser {
				userExists = true
				break
			}
		}

		if !userExists {
			return fmt.Errorf("user %s specified in git config gh.user is not authenticated for %s", configuredUser, hostname)
		}

		// Switch to the configured user
		err = authCfg.SwitchUser(hostname, configuredUser)
		if err != nil {
			return fmt.Errorf("failed to switch to user %s: %w", configuredUser, err)
		}
	}

	return nil
}

// GetGitConfigUser returns the gh.user setting from git config, if any
func GetGitConfigUser() (string, error) {
	gitClient := &git.Client{}
	ctx := context.Background()
	
	// Check if we're in a git repo
	isRepo, err := gitClient.IsLocalGitRepo(ctx)
	if err != nil || !isRepo {
		return "", nil
	}

	// Try to read gh.user from git config  
	return gitClient.Config(ctx, "gh.user")
}

// SetGitConfigUser sets the gh.user setting in git config
func SetGitConfigUser(username string) error {
	gitClient := &git.Client{}
	ctx := context.Background()
	
	// Check if we're in a git repo
	isRepo, err := gitClient.IsLocalGitRepo(ctx)
	if err != nil || !isRepo {
		return fmt.Errorf("not in a git repository")
	}

	// Set gh.user in git config
	cmd, err := gitClient.Command(ctx, "config", "gh.user", username)
	if err != nil {
		return err
	}
	
	_, err = cmd.Output()
	return err
}