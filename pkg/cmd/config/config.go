package config

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration for gh",
		Long: heredoc.Doc(`
			Display or change configuration settings for gh.
	
			Current respected settings:
			- git_protocol: "https" or "ssh". Default is "https".
			- editor: if unset, defaults to environment variables.
			- prompt: "enabled" or "disabled". Toggles interactive prompting.
			- pager: terminal pager program to send standard output to.
		`),
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
			$ gh config set git_protocol ssh
			$ gh config set prompt disabled
		`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			key, value := args[0], args[1]
			err = cfg.Set(hostname, key, value)
			if err != nil {
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
