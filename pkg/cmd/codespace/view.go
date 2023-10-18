package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
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
		Example: heredoc.Doc(`
			# select a codespace from a list of all codespaces you own
			$ gh cs view	

			# view the details of a specific codespace
			$ gh cs view -c codespace-name-12345

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

	//nolint:staticcheck // SA1019: Showing NAME|VALUE headers adds nothing to table.
	tp := tableprinter.New(a.io, tableprinter.NoHeader)
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
		// Don't display the field if it is empty and we are printing to a TTY
		if !a.io.IsStdoutTTY() || field.value != "" {
			tp.AddField(field.name)
			tp.AddField(field.value)
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

	// Format the commits ahead/behind with proper pluralization
	commitsAhead := text.Pluralize(codespace.GitStatus.Ahead, "commit")
	commitsBehind := text.Pluralize(codespace.GitStatus.Behind, "commit")

	return fmt.Sprintf("%s - %s ahead, %s behind", branchWithGitStatus, commitsAhead, commitsBehind)
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
