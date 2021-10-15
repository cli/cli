package set

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type SetOptions struct {
	IO     *iostreams.IOStreams
	Config config.Config

	Key      string
	Value    string
	Hostname string
}

func NewCmdConfigSet(f *cmdutil.Factory, runF func(*SetOptions) error) *cobra.Command {
	opts := &SetOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update configuration with a value for the given key",
		Example: heredoc.Doc(`
			$ gh config set editor vim
			$ gh config set editor "code --wait"
			$ gh config set git_protocol ssh --host github.com
			$ gh config set prompt disabled
		`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := f.Config()
			if err != nil {
				return err
			}
			opts.Config = config
			opts.Key = args[0]
			opts.Value = args[1]

			if runF != nil {
				return runF(opts)
			}

			return setRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Hostname, "host", "h", "", "Set per-host setting")

	return cmd
}

func setRun(opts *SetOptions) error {
	err := config.ValidateKey(opts.Key)
	if err != nil {
		warningIcon := opts.IO.ColorScheme().WarningIcon()
		fmt.Fprintf(opts.IO.ErrOut, "%s warning: '%s' is not a known configuration key\n", warningIcon, opts.Key)
	}

	err = config.ValidateValue(opts.Key, opts.Value)
	if err != nil {
		var invalidValue *config.InvalidValueError
		if errors.As(err, &invalidValue) {
			var values []string
			for _, v := range invalidValue.ValidValues {
				values = append(values, fmt.Sprintf("'%s'", v))
			}
			return fmt.Errorf("failed to set %q to %q: valid values are %v", opts.Key, opts.Value, strings.Join(values, ", "))
		}
	}

	err = opts.Config.Set(opts.Hostname, opts.Key, opts.Value)
	if err != nil {
		return fmt.Errorf("failed to set %q to %q: %w", opts.Key, opts.Value, err)
	}

	err = opts.Config.Write()
	if err != nil {
		return fmt.Errorf("failed to write config to disk: %w", err)
	}
	return nil
}
