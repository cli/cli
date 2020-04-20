package command

import (
	"fmt"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)

	configGetCmd.Flags().StringP("host", "h", "", "Get per-host setting")
	configSetCmd.Flags().StringP("host", "h", "", "Set per-host setting")

}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Set and get gh settings",
	Long: `Get and set key/value strings. They can be optionally scoped to a particular host.

Current respected settings:
- git_protocol: https or ssh. Default is https.
- editor: if unset, defaults to environment variables.
`,
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Prints the value of a given configuration key.",
	Long: `Get the value for a given configuration key, optionally scoping by hostname.

Examples:
  gh config get git_protocol
  https
  gh config get --host example.com
  ssh
`,
	Args: cobra.ExactArgs(1),
	RunE: configGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Updates configuration with the value of a given key.",
	Long: `Update the configuration by setting a key to a value, optionally scoping by hostname.

Examples:
  gh config set editor vim
  gh config set --host example.com editor eclipse
`,
	Args: cobra.ExactArgs(2),
	RunE: configSet,
}

func configGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	hostname, err := cmd.Flags().GetString("host")
	if err != nil {
		return err
	}

	ctx := contextForCommand(cmd)

	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	val, err := cfg.Get(hostname, key)
	if err != nil {
		return err
	}

	if val != "" {
		out := colorableOut(cmd)
		fmt.Fprintf(out, "%s\n", val)
	}

	return nil
}

func configSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	hostname, err := cmd.Flags().GetString("host")
	if err != nil {
		return err
	}

	ctx := contextForCommand(cmd)

	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	err = cfg.Set(hostname, key, value)
	if err != nil {
		return fmt.Errorf("failed to set %q to %q: %w", key, value, err)
	}

	err = cfg.Write()
	if err != nil {
		return fmt.Errorf("failed to write config to disk: %w", err)
	}

	return nil
}
