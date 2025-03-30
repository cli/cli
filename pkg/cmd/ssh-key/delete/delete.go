package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)

	KeyID     string
	Confirmed bool
	Prompter  prompter.Prompter
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an SSH key from your GitHub account",
		Args:  cmdutil.ExactArgs(1, "cannot delete: key id required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.KeyID = args[0]

			if !opts.IO.CanPrompt() && !opts.Confirmed {
				return cmdutil.FlagErrorf("--yes required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "Skip the confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVarP(&opts.Confirmed, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()
	key, err := getSSHKey(httpClient, host, opts.KeyID)
	if err != nil {
		return err
	}

	if !opts.Confirmed {
		if err := opts.Prompter.ConfirmDeletion(key.Title); err != nil {
			return err
		}
	}

	err = deleteSSHKey(httpClient, host, opts.KeyID)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s SSH key %q (%s) deleted from your account\n", cs.SuccessIcon(), key.Title, opts.KeyID)
	}
	return nil
}
