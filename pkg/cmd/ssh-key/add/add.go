package add

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
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
	Type    string
}

func NewCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "add [<key-file>]",
		Short: "Add an SSH key to your GitHub account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if opts.IO.IsStdoutTTY() && opts.IO.IsStdinTTY() {
					return cmdutil.FlagErrorf("public key file missing")
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

	typeEnums := []string{shared.AuthenticationKey, shared.SigningKey}
	cmdutil.StringEnumFlag(cmd, &opts.Type, "type", "", shared.AuthenticationKey, typeEnums, "Type of the ssh key")
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

	var uploaded bool

	if opts.Type == shared.SigningKey {
		uploaded, err = SSHSigningKeyUpload(httpClient, hostname, keyReader, opts.Title)
	} else {
		uploaded, err = SSHKeyUpload(httpClient, hostname, keyReader, opts.Title)
	}

	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	if uploaded {
		fmt.Fprintf(opts.IO.ErrOut, "%s Public key added to your account\n", cs.SuccessIcon())
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "%s Public key already exists on your account\n", cs.SuccessIcon())
	}

	return nil
}
