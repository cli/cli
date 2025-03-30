package clone

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type CloneOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams

	GitArgs   []string
	Directory string
	Gist      string
}

func NewCmdClone(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &CloneOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "clone <gist> [<directory>] [-- <gitflags>...]",
		Args:  cmdutil.MinimumArgs(1, "cannot clone: gist argument required"),
		Short: "Clone a gist locally",
		Long: heredoc.Docf(`
			Clone a GitHub gist locally.

			A gist can be supplied as argument in either of the following formats:
			- by ID, e.g. %[1]s5b0e0062eb8e9654adad7bb1d81cc75f%[1]s
			- by URL, e.g. %[1]shttps://gist.github.com/OWNER/5b0e0062eb8e9654adad7bb1d81cc75f%[1]s

			Pass additional %[1]sgit clone%[1]s flags by listing them after %[1]s--%[1]s.
		`, "`"),
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
		return cmdutil.FlagErrorf("%w\nSeparate git clone flags with '--'.", err)
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
		hostname, _ := cfg.Authentication().DefaultHost()
		protocol := cfg.GitProtocol(hostname).Value
		gistURL = formatRemoteURL(hostname, gistURL, protocol)
	}

	_, err := opts.GitClient.Clone(context.Background(), gistURL, opts.GitArgs)
	if err != nil {
		return err
	}

	return nil
}

func formatRemoteURL(hostname string, gistID string, protocol string) string {
	if ghauth.IsEnterprise(hostname) {
		if protocol == "ssh" {
			return fmt.Sprintf("git@%s:gist/%s.git", hostname, gistID)
		}
		return fmt.Sprintf("https://%s/gist/%s.git", hostname, gistID)
	}

	if protocol == "ssh" {
		return fmt.Sprintf("git@gist.%s:%s.git", hostname, gistID)
	}
	return fmt.Sprintf("https://gist.%s/%s.git", hostname, gistID)
}
