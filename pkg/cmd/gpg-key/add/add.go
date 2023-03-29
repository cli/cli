package add

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type AddOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HTTPClient func() (*http.Client, error)

	KeyFile string
	Title   string
}

func NewCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "add [<key-file>]",
		Short: "Add a GPG key to your GitHub account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY() {
					return cmdutil.FlagErrorf("GPG key file missing")
				}
				opts.KeyFile = "-"
			} else {
				opts.KeyFile = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return runAdd(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the new key")
	return cmd
}

func runAdd(opts *AddOptions) error {
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

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname, _ := cfg.Authentication().DefaultHost()

	err = gpgKeyUpload(httpClient, hostname, keyReader, opts.Title)
	if err != nil {
		cs := opts.IO.ColorScheme()
		if errors.Is(err, errScopesMissing) {
			fmt.Fprint(opts.IO.ErrOut, "Error: insufficient OAuth scopes to list GPG keys\n")
			fmt.Fprintf(opts.IO.ErrOut, "Run the following to grant scopes: %s\n", cs.Bold("gh auth refresh -s write:gpg_key"))
			return cmdutil.SilentError
		}
		if errors.Is(err, errDuplicateKey) {
			fmt.Fprintf(opts.IO.ErrOut, "%s Error: the key already exists in your account\n", cs.FailureIcon())
			return cmdutil.SilentError
		}
		if errors.Is(err, errWrongFormat) {
			fmt.Fprint(opts.IO.ErrOut, heredoc.Docf(`
				%s Error: the GPG key you are trying to upload might not be in ASCII-armored format.
				Find your GPG key ID with:    %s
				Then add it to your account:  %s
			`, cs.FailureIcon(), cs.Bold("gpg --list-keys"), cs.Bold("gpg --armor --export <ID> | gh gpg-key add -")))
			return cmdutil.SilentError
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s GPG key added to your account\n", cs.SuccessIcon())
	}
	return nil
}
