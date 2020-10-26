package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	longDoc := strings.Builder{}
	longDoc.WriteString("Display or change configuration settings for gh.\n\n")
	longDoc.WriteString("Current respected settings:\n")
	for _, co := range config.ConfigOptions() {
		longDoc.WriteString(fmt.Sprintf("- %s: %s", co.Key, co.Description))
		if co.DefaultValue != "" {
			longDoc.WriteString(fmt.Sprintf(" (default: %q)", co.DefaultValue))
		}
		longDoc.WriteRune('\n')
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration for gh",
		Long:  longDoc.String(),
	}

	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(NewCmdConfigGet(f))
	cmd.AddCommand(NewCmdConfigSet(f))

	return cmd
}

func NewCmdConfigGet(f *cmdutil.Factory) *cobra.Command {
	var hostname string

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the value of a given configuration key",
		Example: heredoc.Doc(`
			$ gh config get git_protocol
			https
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			val, err := cfg.Get(hostname, args[0])
			if err != nil {
				return err
			}

			if val != "" {
				fmt.Fprintf(f.IOStreams.Out, "%s\n", val)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&hostname, "host", "h", "", "Get per-host setting")

	return cmd
}

func NewCmdConfigSet(f *cmdutil.Factory) *cobra.Command {
	var hostname string

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
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			key, value := args[0], args[1]
			knownKey := false
			for _, configKey := range config.ConfigOptions() {
				if key == configKey.Key {
					knownKey = true
					break
				}
			}
			if !knownKey {
				iostreams := f.IOStreams
				warningIcon := iostreams.ColorScheme().WarningIcon()
				fmt.Fprintf(iostreams.ErrOut, "%s warning: '%s' is not a known configuration key\n", warningIcon, key)
			}
			err = cfg.Set(hostname, key, value)
			if err != nil {
				var invalidValue *config.InvalidValueError
				if errors.As(err, &invalidValue) {
					var values []string
					for _, v := range invalidValue.ValidValues {
						values = append(values, fmt.Sprintf("'%s'", v))
					}
					return fmt.Errorf("failed to set %q to %q: valid values are %v", key, value, strings.Join(values, ", "))
				}
				return fmt.Errorf("failed to set %q to %q: %w", key, value, err)
			}

			err = cfg.Write()
			if err != nil {
				return fmt.Errorf("failed to write config to disk: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&hostname, "host", "h", "", "Set per-host setting")

	return cmd
}
