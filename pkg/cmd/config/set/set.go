package set

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type SetOptions struct {
	IO     *iostreams.IOStreams
	Config gh.Config

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
			$ gh config set api_host api-gateway.example.com --host example.com
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
	if err := validateScope(opts.Key, opts.Hostname); err != nil {
		return cmdutil.FlagErrorf("%s", err)
	}

	err := ValidateKey(opts.Key)
	if err != nil {
		warningIcon := opts.IO.ColorScheme().WarningIcon()
		fmt.Fprintf(opts.IO.ErrOut, "%s warning: '%s' is not a known configuration key\n", warningIcon, opts.Key)
	}

	err = ValidateValue(opts.Key, opts.Value)
	if err != nil {
		if invalidValue, ok := errors.AsType[InvalidValueError](err); ok {
			var values []string
			for _, v := range invalidValue.ValidValues {
				values = append(values, fmt.Sprintf("'%s'", v))
			}
			return fmt.Errorf("failed to set %q to %q: valid values are %v", opts.Key, opts.Value, strings.Join(values, ", "))
		}
	}

	opts.Config.Set(opts.Hostname, opts.Key, opts.Value)

	err = opts.Config.Write()
	if err != nil {
		return fmt.Errorf("failed to write config to disk: %w", err)
	}
	return nil
}

func ValidateKey(key string) error {
	for _, configKey := range config.Options {
		if key == configKey.Key {
			return nil
		}
	}

	return fmt.Errorf("invalid key")
}

func validateScope(key, hostname string) error {
	for _, configKey := range config.Options {
		if key == configKey.Key {
			switch {
			case configKey.Scope == config.ConfigScopeHostOnly && hostname == "":
				return fmt.Errorf("--host required when setting %s", key)
			case configKey.Scope == config.ConfigScopeGlobalOnly && hostname != "":
				return fmt.Errorf("--host cannot be used when setting %s", key)
			default:
				return nil
			}
		}
	}

	return nil
}

type InvalidValueError struct {
	ValidValues []string
}

func (e InvalidValueError) Error() string {
	return "invalid value"
}

func ValidateValue(key, value string) error {
	var validValues []string

	for _, v := range config.Options {
		if v.Key == key {
			validValues = v.AllowedValues
			break
		}
	}

	if validValues == nil {
		return nil
	}

	if slices.Contains(validValues, value) {
		return nil
	}

	return InvalidValueError{ValidValues: validValues}
}
