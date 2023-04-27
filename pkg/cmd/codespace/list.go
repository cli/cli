package codespace

import (
	"context"
	"fmt"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type listOptions struct {
	limit    int
	repo     string
	orgName  string
	userName string
	useWeb   bool
}

func newListCmd(app *App) *cobra.Command {
	opts := &listOptions{}
	var exporter cmdutil.Exporter

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List codespaces",
		Long: heredoc.Doc(`
			List codespaces of the authenticated user.

			Alternatively, organization administrators may list all codespaces billed to the organization.
		`),
		Aliases: []string{"ls"},
		Args:    noArgsConstraint,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive(
				"using `--org` or `--user` with `--repo` is not allowed",
				opts.repo != "",
				opts.orgName != "" || opts.userName != "",
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"using `--web` with `--org` or `--user` is not supported, please use with `--repo` instead",
				opts.useWeb,
				opts.orgName != "" || opts.userName != "",
			); err != nil {
				return err
			}

			if opts.limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.limit)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.List(cmd.Context(), opts, exporter)
		},
	}

	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", 30, "Maximum number of codespaces to list")
	listCmd.Flags().StringVarP(&opts.repo, "repo", "R", "", "Repository name with owner: user/repo")
	if err := addDeprecatedRepoShorthand(listCmd, &opts.repo); err != nil {
		fmt.Fprintf(app.io.ErrOut, "%v\n", err)
	}

	listCmd.Flags().StringVarP(&opts.orgName, "org", "o", "", "The `login` handle of the organization to list codespaces for (admin-only)")
	listCmd.Flags().StringVarP(&opts.userName, "user", "u", "", "The `username` to list codespaces for (used with --org)")
	cmdutil.AddJSONFlags(listCmd, &exporter, api.CodespaceFields)

	listCmd.Flags().BoolVarP(&opts.useWeb, "web", "w", false, "List codespaces in the web browser, cannot be used with --user or --org")

	return listCmd
}

func (a *App) List(ctx context.Context, opts *listOptions, exporter cmdutil.Exporter) error {
	if opts.useWeb && opts.repo == "" {
		return a.browser.Browse("https://github.com/codespaces")
	}

	var codespaces []*api.Codespace
	err := a.RunWithProgress("Fetching codespaces", func() (err error) {
		codespaces, err = a.apiClient.ListCodespaces(ctx, api.ListCodespacesOptions{Limit: opts.limit, RepoName: opts.repo, OrgName: opts.orgName, UserName: opts.userName})
		return
	})
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	hasNonProdVSCSTarget := false
	for _, apiCodespace := range codespaces {
		if apiCodespace.VSCSTarget != "" && apiCodespace.VSCSTarget != api.VSCSTargetProduction {
			hasNonProdVSCSTarget = true
			break
		}
	}

	if err := a.io.StartPager(); err != nil {
		a.errLogger.Printf("error starting pager: %v", err)
	}
	defer a.io.StopPager()

	if exporter != nil {
		return exporter.Write(a.io, codespaces)
	}

	if len(codespaces) == 0 {
		return cmdutil.NewNoResultsError("no codespaces found")
	}

	if opts.useWeb && codespaces[0].Repository.ID > 0 {
		return a.browser.Browse(fmt.Sprintf("https://github.com/codespaces?repository_id=%d", codespaces[0].Repository.ID))
	}

	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	tp := utils.NewTablePrinter(a.io)
	if tp.IsTTY() {
		tp.AddField("NAME", nil, nil)
		tp.AddField("DISPLAY NAME", nil, nil)
		if opts.orgName != "" {
			tp.AddField("OWNER", nil, nil)
		}
		tp.AddField("REPOSITORY", nil, nil)
		tp.AddField("BRANCH", nil, nil)
		tp.AddField("STATE", nil, nil)
		tp.AddField("CREATED AT", nil, nil)

		if hasNonProdVSCSTarget {
			tp.AddField("VSCS TARGET", nil, nil)
		}

		tp.EndRow()
	}

	cs := a.io.ColorScheme()
	for _, apiCodespace := range codespaces {
		c := codespace{apiCodespace}

		var stateColor func(string) string
		switch c.State {
		case api.CodespaceStateStarting:
			stateColor = cs.Yellow
		case api.CodespaceStateAvailable:
			stateColor = cs.Green
		}

		formattedName := formatNameForVSCSTarget(c.Name, c.VSCSTarget)

		var nameColor func(string) string
		switch c.PendingOperation {
		case false:
			nameColor = cs.Yellow
		case true:
			nameColor = cs.Gray
		}

		tp.AddField(formattedName, nil, nameColor)
		tp.AddField(c.DisplayName, nil, nil)
		if opts.orgName != "" {
			tp.AddField(c.Owner.Login, nil, nil)
		}
		tp.AddField(c.Repository.FullName, nil, nil)
		tp.AddField(c.branchWithGitStatus(), nil, cs.Cyan)
		if c.PendingOperation {
			tp.AddField(c.PendingOperationDisabledReason, nil, nameColor)
		} else {
			tp.AddField(c.State, nil, stateColor)
		}

		if tp.IsTTY() {
			ct, err := time.Parse(time.RFC3339, c.CreatedAt)
			if err != nil {
				return fmt.Errorf("error parsing date %q: %w", c.CreatedAt, err)
			}
			tp.AddField(text.FuzzyAgoAbbr(time.Now(), ct), nil, cs.Gray)
		} else {
			tp.AddField(c.CreatedAt, nil, nil)
		}

		if hasNonProdVSCSTarget {
			tp.AddField(c.VSCSTarget, nil, nil)
		}

		tp.EndRow()
	}

	return tp.Render()
}

func formatNameForVSCSTarget(name, vscsTarget string) string {
	if vscsTarget == api.VSCSTargetDevelopment || vscsTarget == api.VSCSTargetLocal {
		return fmt.Sprintf("%s 🚧", name)
	}

	if vscsTarget == api.VSCSTargetPPE {
		return fmt.Sprintf("%s ✨", name)
	}

	return name
}
