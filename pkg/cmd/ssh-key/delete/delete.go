package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
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
				return cmdutil.FlagErrorf("--confirm required when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Confirmed, "confirm", "y", false, "Skip the confirmation prompt")
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

	if opts.IO.CanPrompt() && !opts.Confirmed {
		confirmed, err := opts.Prompter.Confirm("Confirm deletion:", true)
		if err != nil {
			return err
		}
		if !confirmed {
			return cmdutil.CancelError
		}
	}

	host, _ := cfg.DefaultHost()

	err = deleteSSHKey(httpClient, host, opts.KeyID)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return fmt.Errorf("unable to delete SSH key id %s: either the SSH key is not found or it is not owned by you", opts.KeyID)
		}

		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s Deleted SSH key id %s from your account\n", cs.SuccessIcon(), opts.KeyID)
	}
	return nil
}
