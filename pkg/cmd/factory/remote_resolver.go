package factory

import (
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
)

type remoteResolver struct {
	readRemotes   func() (git.RemoteSet, error)
	getConfig     func() (config.Config, error)
	urlTranslator func(*url.URL) *url.URL
}

func (rr *remoteResolver) Resolver(hostOverride string) func() (context.Remotes, error) {
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

		knownHosts := map[string]bool{}
		knownHosts[ghinstance.Default()] = true
		if authenticatedHosts, err := cfg.Hosts(); err == nil {
			for _, h := range authenticatedHosts {
				knownHosts[h] = true
			}
		}

		// filter remotes to only those sharing a single, known hostname
		var hostname string
		cachedRemotes = context.Remotes{}
		sort.Sort(resolvedRemotes)

		if hostOverride != "" {
			for _, r := range resolvedRemotes {
				if strings.EqualFold(r.RepoHost(), hostOverride) {
					cachedRemotes = append(cachedRemotes, r)
				}
			}

			if len(cachedRemotes) == 0 {
				remotesError = errors.New("none of the git remotes configured for this repository correspond to the GH_HOST environment variable. Try adding a matching remote or unsetting the variable.")
				return nil, remotesError
			}

			return cachedRemotes, nil
		}

		for _, r := range resolvedRemotes {
			if hostname == "" {
				if !knownHosts[r.RepoHost()] {
					continue
				}
				hostname = r.RepoHost()
			} else if r.RepoHost() != hostname {
				continue
			}
			cachedRemotes = append(cachedRemotes, r)
		}

		if len(cachedRemotes) == 0 {
			remotesError = errors.New("none of the git remotes configured for this repository point to a known GitHub host. To tell gh about a new GitHub host, please use `gh auth login`")
			return nil, remotesError
		}
		return cachedRemotes, nil
	}
}
