package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)

	KeyID     string
	Type      string
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
		Long: `Delete an SSH key from your GitHub account by ID.

Authentication and signing keys are looked up automatically. If the same ID
exists in both namespaces, pass --type to choose which key to delete.
`,
		Args: cmdutil.ExactArgs(1, "cannot delete: key id required"),
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

	typeEnums := []string{shared.AuthenticationKey, shared.SigningKey}
	cmdutil.StringEnumFlag(cmd, &opts.Type, "type", "", "", typeEnums, "Type of the ssh key (required only when the ID exists as both authentication and signing)")
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
	key, err := resolveSSHKey(httpClient, host, opts.KeyID, opts.Type)
	if err != nil {
		var ambiguous ambiguousKeyError
		if errors.As(err, &ambiguous) {
			key, err = disambiguateKey(opts, ambiguous)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if !opts.Confirmed {
		confirmLabel := key.Title
		if key.Type != "" {
			confirmLabel = fmt.Sprintf("%s (%s)", key.Title, key.Type)
		}
		if err := opts.Prompter.ConfirmDeletion(confirmLabel); err != nil {
			return err
		}
	}

	err = deleteSSHKey(httpClient, host, opts.KeyID, key.Type)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s SSH key %q (%s) deleted from your account\n", cs.SuccessIcon(), key.Title, opts.KeyID)
	}
	return nil
}

func disambiguateKey(opts *DeleteOptions, ambiguous ambiguousKeyError) (*sshKey, error) {
	if !opts.IO.CanPrompt() {
		return nil, ambiguous
	}

	options := []string{
		fmt.Sprintf("%s (%s)", ambiguous.AuthTitle, shared.AuthenticationKey),
		fmt.Sprintf("%s (%s)", ambiguous.SigningTitle, shared.SigningKey),
	}
	idx, err := opts.Prompter.Select(
		fmt.Sprintf("SSH key ID %s matches both an authentication and a signing key. Which one should be deleted?", ambiguous.KeyID),
		"",
		options,
	)
	if err != nil {
		return nil, err
	}

	switch idx {
	case 0:
		return &sshKey{Title: ambiguous.AuthTitle, Type: shared.AuthenticationKey}, nil
	case 1:
		return &sshKey{Title: ambiguous.SigningTitle, Type: shared.SigningKey}, nil
	default:
		return nil, fmt.Errorf("invalid selection")
	}
}
