package codespace

import (
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

// newPortsCmd returns a Cobra "ports" command that displays a table of available ports,
// according to the specified flags.
func newPortsCmd(app *codespaces.App) *cobra.Command {
	var (
		codespace string
		asJSON    bool
	)

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "List ports in a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ListPorts(cmd.Context(), codespace, asJSON)
		},
	}

	portsCmd.PersistentFlags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	portsCmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	portsCmd.AddCommand(newPortsForwardCmd(app))
	portsCmd.AddCommand(newPortsPrivacyCmd(app))

	return portsCmd
}

func newPortsPrivacyCmd(app *codespaces.App) *cobra.Command {
	return &cobra.Command{
		Use:     "privacy <port:public|private|org>...",
		Short:   "Change the privacy of the forwarded port",
		Example: "gh codespace ports privacy 80:org 3000:private 8000:public",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("at least one port privacy argument is required")
			}
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %w", err)
			}
			return app.UpdatePortPrivacy(cmd.Context(), codespace, args)
		},
	}
}

// NewPortsForwardCmd returns a Cobra "ports forward" subcommand, which forwards a set of
// port pairs from the codespace to localhost.
func newPortsForwardCmd(app *codespaces.App) *cobra.Command {
	return &cobra.Command{
		Use:   "forward <remote-port>:<local-port>...",
		Short: "Forward ports",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			codespace, err := cmd.Flags().GetString("codespace")
			if err != nil {
				// should only happen if flag is not defined
				// or if the flag is not of string type
				// since it's a persistent flag that we control it should never happen
				return fmt.Errorf("get codespace flag: %w", err)
			}

			return app.ForwardPorts(cmd.Context(), codespace, args)
		},
	}
}
