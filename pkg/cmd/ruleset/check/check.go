package check

import (
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)

	Branch string
}

func NewCmdCheck(f *cmdutil.Factory, runF func(*CheckOptions) error) *cobra.Command {
	opts := &CheckOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}
	cmd := &cobra.Command{
		Use:   "check [<branch>]",
		Short: "Print rules that would apply to a given branch",
		Long: heredoc.Doc(`
			TODO
		`),
		Example: "TODO",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			// TODO flag to do a push

			if len(args) > 0 {
				opts.Branch = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return checkRun(opts)
		},
	}

	return cmd
}

func checkRun(opts *CheckOptions) error {
	// TODO sniff local branch if opts.Branch is empty
	// TODO ask about pushing (if interactive)
	// TODO error if not interactive and --push not specified

	// is the --push redundant? like, it needs to be specified every time for scripted use. can i tell if a branch is up to date with remote without a push? i could figure that out i think and then would know a push wasn't needed (but it will require a fetch per invocation. that seems fine?)
	return nil
}
