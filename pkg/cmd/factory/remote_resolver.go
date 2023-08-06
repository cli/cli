package factory

import (
	"errors"
	"fmt"
	"sort"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/cli/go-gh/v2/pkg/ssh"
)

const (
	GH_HOST = "GH_HOST"
)

type remoteResolver struct {
	readRemotes   func() (git.RemoteSet, error)
	getConfig     func() (config.Config, error)
	urlTranslator context.Translator
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
			sshTranslate = ssh.NewTranslator()
		}
		resolvedRemotes := context.TranslateRemotes(gitRemotes, sshTranslate)

		cfg, err := rr.getConfig()
		if err != nil {
			return nil, err
		}

		authedHosts := cfg.Authentication().Hosts()
		if len(authedHosts) == 0 {
			return nil, errors.New("could not find any host configurations")
		}
		defaultHost, src := cfg.Authentication().DefaultHost()

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
		// For environment default host (GH_HOST) do not fallback to cachedRemotes if none match
		if src != "default" {
			filteredRemotes := cachedRemotes.FilterByHosts([]string{defaultHost})
			if isHostEnv(src) || len(filteredRemotes) > 0 {
				cachedRemotes = filteredRemotes
			}
		}

		if len(cachedRemotes) == 0 {
			if isHostEnv(src) {
				return nil, fmt.Errorf("none of the git remotes configured for this repository correspond to the %s environment variable. Try adding a matching remote or unsetting the variable.", src)
			} else if cfg.Authentication().HasEnvToken() {
				return nil, errors.New("set the GH_HOST environment variable to specify which GitHub host to use")
			}
			return nil, errors.New("none of the git remotes configured for this repository point to a known GitHub host. To tell gh about a new GitHub host, please use `gh auth login`")
		}

		return cachedRemotes, nil
	}
}

func isHostEnv(src string) bool {
	return src == GH_HOST
}
