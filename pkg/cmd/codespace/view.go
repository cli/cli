package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

const (
	minutesInDay = 1440
)

type viewOptions struct {
	selector *CodespaceSelector
	exporter cmdutil.Exporter
}

func newViewCmd(app *App) *cobra.Command {
	opts := &viewOptions{}

	viewCmd := &cobra.Command{
		Use:   "view",
		Short: "View details about a codespace",
		Long: heredoc.Doc(`
			View details about a codespace.

			For more fine-grained details, you can add the "--json" flag to view the full list of fields available.

			If this command doesn't provide enough information, you can use the "gh api" command to view the full JSON response:
			$ gh api /user/codespaces/<codespace-name>
		`),
		Example: heredoc.Doc(`
			# select a codespace from a list of all codespaces you own
			$ gh cs view	

			# view the details of a specific codespace
			$ gh cs view -c <codespace-name>
			
			# select a codespace from a list of codespaces in a repository
			$ gh cs view --repo <owner>/<repo>

			# select a codespace from a list of codespaces with a specific repository owner
			$ gh cs view --repo-owner <org/username>

			# view the list of all available fields for a codespace
			$ gh cs view --json

			# view specific fields for a codespace
			$ gh cs view --json displayName,machineDisplayName,state
		`),
		Args: noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ViewCodespace(cmd.Context(), opts)
		},
	}
	opts.selector = AddCodespaceSelector(viewCmd, app.apiClient)
	cmdutil.AddJSONFlags(viewCmd, &opts.exporter, api.ViewCodespaceFields)

	return viewCmd
}

func (a *App) ViewCodespace(ctx context.Context, opts *viewOptions) error {
	// If we are in a codespace and a codespace name wasn't provided, show the details for the codespace we are connected to
	if (os.Getenv("CODESPACES") == "true") && opts.selector.codespaceName == "" {
		codespaceName := os.Getenv("CODESPACE_NAME")
		opts.selector.codespaceName = codespaceName
	}

	selectedCodespace, err := opts.selector.Select(ctx)
	if err != nil {
		return err
	}

	if err := a.io.StartPager(); err != nil {
		a.errLogger.Printf("error starting pager: %v", err)
	}
	defer a.io.StopPager()

	if opts.exporter != nil {
		return opts.exporter.Write(a.io, selectedCodespace)
	}

	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	tp := utils.NewTablePrinter(a.io)
	c := codespace{selectedCodespace}
	formattedName := formatNameForVSCSTarget(c.Name, c.VSCSTarget)

	// Create an array of fields to display in the table with their values
	fields := []struct {
		name  string
		value string
	}{
		{"Name", formattedName},
		{"State", c.State},
		{"Repository", c.Repository.FullName},
		{"Git Status", formatGitStatus(c)},
		{"Devcontainer Path", c.DevContainerPath},
		{"Machine Display Name", c.Machine.DisplayName},
		{"Idle Timeout", fmt.Sprintf("%d minutes", c.IdleTimeoutMinutes)},
		{"Created At", c.CreatedAt},
		{"Retention Period", formatRetentionPeriodDays(c)},
	}

	for _, field := range fields {
		// Only display the field if it has a value.
		if field.value != "" {
			tp.AddField(field.name, nil, nil)
			tp.AddField(field.value, nil, nil)
			tp.EndRow()
		}
	}

	err = tp.Render()
	if err != nil {
		return err
	}

	return nil
}

func formatGitStatus(codespace codespace) string {
	branchWithGitStatus := codespace.branchWithGitStatus()
	// u2193 and u2191 are the unicode arrows for down and up
	return fmt.Sprintf("%s - %d\u2193 %d\u2191", branchWithGitStatus, codespace.GitStatus.Behind, codespace.GitStatus.Ahead)
}

func formatRetentionPeriodDays(codespace codespace) string {
	days := codespace.RetentionPeriodMinutes / minutesInDay
	// Don't display the retention period if it is 0 days
	if days == 0 {
		return ""
	} else if days == 1 {
		return "1 day"
	}

	return fmt.Sprintf("%d days", days)
}
