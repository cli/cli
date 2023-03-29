package add

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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

	keyBytes, err := KeyBytes(keyReader)
	if err != nil {
		return err
	}

	userKey := string(keyBytes)
	splitKey := strings.Fields(userKey)
	if len(splitKey) < 2 {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Error: provided key is not in a valid format\n", cs.FailureIcon())
		return cmdutil.SilentError
	}

	userKey = splitKey[0] + " " + splitKey[1]

	keys, err := shared.UserKeys(httpClient, hostname, "")
	if err != nil {
		return err
	}

	for _, k := range keys {
		if k.Key == userKey {
			printSuccess(opts, "Key already exists on your account, no action taken")
			return nil
		}
	}

	err = SSHKeyUpload(httpClient, hostname, keyBytes, opts.Title)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		printSuccess(opts, "Public key added to your account")
	}
	return nil
}

func KeyBytes(keyReader io.Reader) ([]byte, error) {
	return io.ReadAll(keyReader)
}

func printSuccess(opts *AddOptions, msg string) {
	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out, "%s %s\n", cs.SuccessIcon(), msg)
}
