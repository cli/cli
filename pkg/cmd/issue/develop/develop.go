package develop

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DevelopOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	IssueRepo  string
	Name       string
	BaseBranch string
	Checkout   bool
}

func NewCmdDevelop(f *cmdutil.Factory, runF func(*DevelopOptions) error) *cobra.Command {
	opts := &DevelopOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "develop",
		Short: "Manage branches for an issue",
		Example: heredoc.Doc(`
			$ gh issue develop --list 123 # list branches for issue 123
			$ gh issue develop --issue-repo "github/cli" 123 list branches for issue 123 in repo "github/cli"
			$ gh issue develop 123 --name "my-branch" --head main
			`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return developRun(opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Name, "name", "n", "", "Name of the branch to create")
	fl.StringVarP(&opts.BaseBranch, "base-branch", "b", "", "Name of the base branch")
	fl.StringVarP(&opts.IssueRepo, "issue-repo", "i", "", "Name of the issue's repository")
	return cmd
}

func developRun(opts *DevelopOptions) (err error) {
	// httpClient, err := opts.HttpClient()
	// if err != nil {
	// 	return err
	// }
	fmt.Fprintf(opts.IO.ErrOut, "hello world")
	return
}
