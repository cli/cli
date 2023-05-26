package codespace

import (
	"context"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type viewOptions struct {
	selector *CodespaceSelector
}

func newViewCmd(app *App) *cobra.Command {
	opts := &viewOptions{}
	var exporter cmdutil.Exporter

	viewCmd := &cobra.Command{
		Use:   "view",
		Short: "View details about a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ViewCodespace(cmd.Context(), opts, exporter)
		},
	}
	opts.selector = AddCodespaceSelector(viewCmd, app.apiClient)
	cmdutil.AddJSONFlags(viewCmd, &exporter, api.ViewCodespaceFields)

	return viewCmd
}

func (a *App) ViewCodespace(ctx context.Context, opts *viewOptions, exporter cmdutil.Exporter) error {
	selectedCodespace, err := opts.selector.Select(ctx)
	if err != nil {
		return err
	}

	if err := a.io.StartPager(); err != nil {
		a.errLogger.Printf("error starting pager: %v", err)
	}
	defer a.io.StopPager()

	if exporter != nil {
		return exporter.Write(a.io, selectedCodespace)
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
		{"Display Name", c.DisplayName},
		{"State", c.State},
		{"Machine Name", c.Machine.Name},
		{"Machine Display Name", c.Machine.DisplayName},
		{"Prebuild", strconv.FormatBool(c.Prebuild)},
		{"Owner", c.Owner.Login},
		{"BillableOwner", c.BillableOwner.Login},
		{"Location", c.Location},
		{"Repository", c.Repository.FullName},
		{"Branch", c.GitStatus.Ref},
		{"Devcontainer Path", c.DevContainerPath},
		{"Commits Ahead", strconv.Itoa(c.GitStatus.Ahead)},
		{"Commits Behind", strconv.Itoa(c.GitStatus.Behind)},
		{"Has Uncommitted Changes", strconv.FormatBool(c.GitStatus.HasUncommittedChanges)},
		{"Has Unpushed Changes", strconv.FormatBool(c.GitStatus.HasUnpushedChanges)},
		{"Created At", c.CreatedAt},
		{"Last Used At", c.LastUsedAt},
		{"Idle Timeout Minutes", strconv.Itoa(c.IdleTimeoutMinutes)},
		{"Retention Expires At", c.RetentionExpiresAt},
		{"Recent Folders", strings.Join(c.RecentFolders, ", ")},
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
