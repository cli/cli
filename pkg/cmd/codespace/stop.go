package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type stopOptions struct {
	codespaceName string
	orgName       string
	userName      string
}

func newStopCmd(app *App) *cobra.Command {
	opts := &stopOptions{}

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.StopCodespace(cmd.Context(), opts)
		},
	}
	stopCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Name of the codespace")
	stopCmd.Flags().StringVarP(&opts.orgName, "org", "o", "", "Stop codespace for an organization member (admin-only)")
	stopCmd.Flags().StringVarP(&opts.userName, "username", "u", "", "Owner of the codespace. Required when using both -c and -o flags.")

	return stopCmd
}

func (a *App) StopCodespace(ctx context.Context, opts *stopOptions) error {
	if opts.codespaceName == "" {
		a.StartProgressIndicatorWithLabel("Fetching codespaces")
		codespaces, err := a.apiClient.ListCodespaces(ctx, -1, opts.orgName)
		a.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to list codespaces: %w", err)
		}

		var runningCodespaces []*api.Codespace
		for _, c := range codespaces {
			cs := codespace{c}
			if cs.running() {
				runningCodespaces = append(runningCodespaces, c)
			}
		}
		if len(runningCodespaces) == 0 {
			return errors.New("no running codespaces")
		}

		includeUsername := opts.orgName != ""
		codespace, err := chooseCodespaceFromList(ctx, runningCodespaces, includeUsername)

		if err != nil {
			return fmt.Errorf("failed to choose codespace: %w", err)
		}
		opts.codespaceName = codespace.Name
		opts.userName = codespace.Owner.Login
	} else {
		a.StartProgressIndicatorWithLabel("Fetching codespace")

		var c *api.Codespace
		var err error

		if opts.orgName == "" {
			c, err = a.apiClient.GetCodespace(ctx, opts.codespaceName, false)
		} else {
			c, err = a.apiClient.GetOrgMemberCodespace(ctx, opts.orgName, opts.userName, opts.codespaceName)
		}
		a.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get codespace: %q: %w", opts.codespaceName, err)
		}
		cs := codespace{c}
		if !cs.running() {
			return fmt.Errorf("codespace %q is not running", opts.codespaceName)
		}
	}

	a.StartProgressIndicatorWithLabel("Stopping codespace")
	defer a.StopProgressIndicator()
	if err := a.apiClient.StopCodespace(ctx, opts.codespaceName, opts.orgName, opts.userName); err != nil {
		return fmt.Errorf("failed to stop codespace: %w", err)
	}

	return nil
}
