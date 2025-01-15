package add

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type AddOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	KeyFile    string
	Title      string
	AllowWrite bool
}

func NewCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{
		HTTPClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "add <key-file>",
		Short: "Add a deploy key to a GitHub repository",
		Long: heredoc.Doc(`
			Add a deploy key to a GitHub repository.
			
			Note that any key added by gh will be associated with the current authentication token.
			If you de-authorize the GitHub CLI app or authentication token from your account, any
			deploy keys added by GitHub CLI will be removed as well.
		`),
		Example: heredoc.Doc(`
			# generate a passwordless SSH key and add it as a deploy key to a repository
			$ ssh-keygen -t ed25519 -C "my description" -N "" -f ~/.ssh/gh-test
			$ gh repo deploy-key add ~/.ssh/gh-test.pub
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.KeyFile = args[0]

			if runF != nil {
				return runF(opts)
			}
			return addRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title of the new key")
	cmd.Flags().BoolVarP(&opts.AllowWrite, "allow-write", "w", false, "Allow write access for the key")
	return cmd
}

func addRun(opts *AddOptions) error {
	httpClient, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	var keyReader io.Reader
	if opts.KeyFile == "-" {
		keyReader = opts.IO.In
		defer opts.IO.In.Close()
	} else {
		f, err := os.Open(opts.KeyFile)
		if err != nil {
			return err
		}
		defer f.Close()
		keyReader = f
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if err := uploadDeployKey(httpClient, repo, keyReader, opts.Title, opts.AllowWrite); err != nil {
		return err
	}

	if !opts.IO.IsStdoutTTY() {
		return nil
	}

	cs := opts.IO.ColorScheme()
	_, err = fmt.Fprintf(opts.IO.Out, "%s Deploy key added to %s\n", cs.SuccessIcon(), cs.Bold(ghrepo.FullName(repo)))
	return err
}
