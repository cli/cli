package factory

import (
	"errors"
	"fmt"
	"net/url"
	"sort"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/set"
)

type remoteResolver struct {
	readRemotes   func() (git.RemoteSet, error)
	getConfig     func() (config.Config, error)
	urlTranslator func(*url.URL) *url.URL
}

func (rr *remoteResolver) Resolver() func() (context.Remotes, error) {
	var cachedRemotes context.Remotes
	var remotesError error

	return func() (context.Remotes, error) {
		if cachedRemotes != nil || remotesError != nil {
			return cachedRemotes, remotesError
		}

		gitRemotes, err := rr.readRemotes()
		if err != nil {
			remotesError = err
			return nil, err
		}
		if len(gitRemotes) == 0 {
			remotesError = errors.New("no git remotes found")
			return nil, remotesError
		}

		sshTranslate := rr.urlTranslator
		if sshTranslate == nil {
			sshTranslate = git.ParseSSHConfig().Translator()
		}
		resolvedRemotes := context.TranslateRemotes(gitRemotes, sshTranslate)

		cfg, err := rr.getConfig()
		if err != nil {
			return nil, err
		}

		authedHosts, err := cfg.Hosts()
		if err != nil {
			return nil, err
		}
		defaultHost, src, err := cfg.DefaultHostWithSource()
		if err != nil {
			return nil, err
		}
		// Use set to dedupe list of hosts
		hostsSet := set.NewStringSet()
		hostsSet.AddValues(authedHosts)
		hostsSet.AddValues([]string{defaultHost, ghinstance.Default()})
		hosts := hostsSet.ToSlice()

		// Sort remotes
		sort.Sort(resolvedRemotes)

		// Filter remotes by hosts
		cachedRemotes := resolvedRemotes.FilterByHosts(hosts)

		// Filter again by default host if one is set
		// For config file default host fallback to cachedRemotes if none match
		// For enviornment default host (GH_HOST) do not fallback to cachedRemotes if none match
		if src != "" {
			filteredRemotes := cachedRemotes.FilterByHosts([]string{defaultHost})
			if config.IsHostEnv(src) || len(filteredRemotes) > 0 {
				cachedRemotes = filteredRemotes
			}
		}

		if len(cachedRemotes) == 0 {
			dummyHostname := "example.com" // any non-github.com hostname is fine here
			if config.IsHostEnv(src) {
				return nil, fmt.Errorf("none of the git remotes configured for this repository correspond to the %s environment variable. Try adding a matching remote or unsetting the variable.", src)
			} else if v, src, _ := cfg.GetWithSource(dummyHostname, "oauth_token"); v != "" && config.IsEnterpriseEnv(src) {
				return nil, errors.New("set the GH_HOST environment variable to specify which GitHub host to use")
			}
			return nil, errors.New("none of the git remotes configured for this repository point to a known GitHub host. To tell gh about a new GitHub host, please use `gh auth login`")
		}

		return cachedRemotes, nil
	}
}
