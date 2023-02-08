package list

import (
	"fmt"

	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/variable/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/cli/cli/v2/internal/tableprinter"
)

func NewCmdList(f *cmdutil.Factory, runF func(*shared.ListOptions) error) *cobra.Command {
	opts := &shared.ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List variables",
		Long: heredoc.Doc(`
			List variables on one of the following levels:
			- repository (default): available to Actions runs in a repository
			- environment: available to Actions runs for a deployment environment in a repository
			- organization: available to Actions runs within an organization
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--org` or `--env`", opts.OrgName != "", opts.EnvName != ""); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.OrgName, "org", "o", "", "List variables for an organization")
	cmd.Flags().StringVarP(&opts.EnvName, "env", "e", "", "List variables for an environment")
	cmd.Flags().IntVar(&opts.Page, "page", 1, "Page Number")
	cmd.Flags().IntVar(&opts.PerPage, "perpage", 10, "Maximum variable count")
	cmd.Flags().StringVarP(&opts.Name, "name", "n", "", "Name of a particular variable")

	return cmd
}

func listRun(opts *shared.ListOptions) error {

	showSelectedRepoInfo := opts.IO.IsStdoutTTY()
	variables, err := shared.GetVariablesForList(opts, showSelectedRepoInfo)
	if err != nil {
		return err
	}

	if len(variables) == 0 {
		return cmdutil.NewNoResultsError("no variables found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	table := tableprinter.New(opts.IO)
	if opts.OrgName != "" {
		table.HeaderRow("NAME", "VALUE", "UPDATED AT", "VISIBILITY")
	} else {
		table.HeaderRow("NAME", "VALUE", "UPDATED AT")
	}
	for _, variable := range variables {
		table.AddField(variable.Name)
		table.AddField(variable.Value)
		updatedAt := variable.UpdatedAt.Format("2006-01-02")
		if opts.IO.IsStdoutTTY() {
			updatedAt = fmt.Sprintf("Updated %s", updatedAt)
		}
		table.AddField(updatedAt)
		if variable.Visibility != "" {
			if showSelectedRepoInfo {
				table.AddField(fmtVisibility(*variable))
			} else {
				table.AddField(strings.ToUpper(string(variable.Visibility)), nil, nil)
			}
		}
		table.EndRow()
	}

	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

func fmtVisibility(s shared.Variable) string {
	switch s.Visibility {
	case shared.All:
		return "Visible to all repositories"
	case shared.Private:
		return "Visible to private repositories"
	case shared.Selected:
		if s.NumSelectedRepos == 1 {
			return "Visible to 1 selected repository"
		} else {
			return fmt.Sprintf("Visible to %d selected repositories", s.NumSelectedRepos)
		}
	}
	return ""
}
