package delete

import (
	"fmt"
	"net/http"
	"strconv"

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
		Use:   "delete <key-id>",
		Short: "Delete a GPG key from your GitHub account",
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
	gpgKeys, err := getGPGKeys(httpClient, host)
	if err != nil {
		return err
	}

	id := ""
	for _, gpgKey := range gpgKeys {
		if gpgKey.KeyID == opts.KeyID {
			id = strconv.Itoa(gpgKey.ID)
			break
		}
	}

	if id == "" {
		return fmt.Errorf("unable to delete GPG key %s: either the GPG key is not found or it is not owned by you", opts.KeyID)
	}

	if !opts.Confirmed {
		if err := opts.Prompter.ConfirmDeletion(opts.KeyID); err != nil {
			return err
		}
	}

	err = deleteGPGKey(httpClient, host, id)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s GPG key %s deleted from your account\n", cs.SuccessIcon(), opts.KeyID)
	}

	return nil
}
