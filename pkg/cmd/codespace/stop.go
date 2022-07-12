package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
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
			if opts.orgName != "" && opts.codespaceName != "" && opts.userName == "" {
				return cmdutil.FlagErrorf("using `--org` with `--codespace` requires `--user`")
			}
			return app.StopCodespace(cmd.Context(), opts)
		},
	}
	stopCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Name of the codespace")
	stopCmd.Flags().StringVarP(&opts.orgName, "org", "o", "", "The `login` handle of the organization (admin-only)")
	stopCmd.Flags().StringVarP(&opts.userName, "user", "u", "", "The `username` to stop codespace for (used with --org)")

	return stopCmd
}

func (a *App) StopCodespace(ctx context.Context, opts *stopOptions) error {
	codespaceName := opts.codespaceName
	ownerName := opts.userName

	if codespaceName == "" {
		a.StartProgressIndicatorWithLabel("Fetching codespaces")
		codespaces, err := a.apiClient.ListCodespaces(ctx, api.ListCodespacesOptions{OrgName: opts.orgName, UserName: ownerName})
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

		includeOwner := opts.orgName != ""
		codespace, err := chooseCodespaceFromList(ctx, runningCodespaces, includeOwner)
		if err != nil {
			return fmt.Errorf("failed to choose codespace: %w", err)
		}
		codespaceName = codespace.Name
		ownerName = codespace.Owner.Login
	} else {
		a.StartProgressIndicatorWithLabel("Fetching codespace")

		var c *api.Codespace
		var err error

		if opts.orgName == "" {
			c, err = a.apiClient.GetCodespace(ctx, codespaceName, false)
		} else {
			c, err = a.apiClient.GetOrgMemberCodespace(ctx, opts.orgName, ownerName, codespaceName)
		}
		a.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("failed to get codespace: %q: %w", codespaceName, err)
		}
		cs := codespace{c}
		if !cs.running() {
			return fmt.Errorf("codespace %q is not running", codespaceName)
		}
	}

	a.StartProgressIndicatorWithLabel("Stopping codespace")
	defer a.StopProgressIndicator()
	if err := a.apiClient.StopCodespace(ctx, codespaceName, opts.orgName, ownerName); err != nil {
		return fmt.Errorf("failed to stop codespace: %w", err)
	}

	return nil
}
