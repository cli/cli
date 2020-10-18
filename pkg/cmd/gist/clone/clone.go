package clone

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CloneOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	GitArgs   []string
	Directory string
	Gist      string
}

func NewCmdClone(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &CloneOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "clone <gist> [<directory>] [-- <gitflags>...]",
		Args:  cmdutil.MinimumArgs(1, "cannot clone: gist argument required"),
		Short: "Clone a gist locally",
		Long: heredoc.Doc(`
			Clone a GitHub gist locally.

			A gist can be supplied as argument in either of the following formats:
			- by ID, e.g. 5b0e0062eb8e9654adad7bb1d81cc75f
			- by URL, e.g. "https://gist.github.com/OWNER/5b0e0062eb8e9654adad7bb1d81cc75f"

			Pass additional 'git clone' flags by listing them after '--'.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Gist = args[0]
			opts.GitArgs = args[1:]

			if runF != nil {
				return runF(opts)
			}

			return cloneRun(opts)
		},
	}

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: fmt.Errorf("%w\nSeparate git clone flags with '--'.", err)}
	})

	return cmd
}

func cloneRun(opts *CloneOptions) error {
	gistURL := opts.Gist

	if !git.IsURL(gistURL) {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		hostname := ghinstance.OverridableDefault()
		protocol, err := cfg.Get(hostname, "git_protocol")
		if err != nil {
			return err
		}
		gistURL = formatRemoteURL(hostname, gistURL, protocol)
	}

	_, err := git.RunClone(gistURL, opts.GitArgs)
	if err != nil {
		return err
	}

	return nil
}

func formatRemoteURL(hostname string, gistID string, protocol string) string {
	if protocol == "ssh" {
		return fmt.Sprintf("git@gist.%s:%s.git", hostname, gistID)
	}

	return fmt.Sprintf("https://gist.%s/%s.git", hostname, gistID)
}
