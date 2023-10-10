package authswitch

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type SwitchOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	Prompter   shared.Prompt
}

func NewCmdSwitch(f *cmdutil.Factory, runF func(*SwitchOptions) error) *cobra.Command {
	opts := &SwitchOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "switch",
		Args:  cobra.ExactArgs(0),
		Short: "Switch between GitHub accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return switchRun(opts)
		},
	}

	return cmd
}

type hostAndUsers struct {
	host  string
	users []string
}

func switchRun(opts *SwitchOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	cs := opts.IO.ColorScheme()

	hosts := authCfg.Hosts()
	var githubUsers []string
	hostsToUsers := make([]hostAndUsers, len(hosts))

	for i, host := range hosts {
		users, err := authCfg.UsersForHost(host)
		if err != nil {
			return fmt.Errorf("failed to get user list for host %q: %s\n", host, err)
		}
		sort.Strings(users)

		hostsToUsers[i] = hostAndUsers{host, users}

		if ghinstance.Default() == host {
			githubUsers = append(githubUsers, users...)
		}
	}

	// sort.Slice(hostsToUsers, func(i, j int) bool {
	// 	return hostsToUsers[i].host < hostsToUsers[j].host
	// })

	// The logic here to offer all hosts and find our way back to the right user and host is
	// awful, so let's just offer GitHub.com for now.
	//
	// It would be nice if we highlighted the currently active user as well.
	selectedUserIdx, err := opts.Prompter.Select("Which account do you want to switch to?", githubUsers[0], githubUsers)
	if err != nil {
		return err
	}

	if err = authCfg.Switch(ghinstance.Default(), githubUsers[selectedUserIdx]); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut,
		"%s Switched to %s on %s\n",
		cs.SuccessIcon(),
		cs.Bold(githubUsers[selectedUserIdx]),
		cs.Bold(ghinstance.Default()),
	)

	return nil
}
