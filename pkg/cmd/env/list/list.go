package list

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/env/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type EnvironmentListClient interface {
	List(repo ghrepo.Interface, limit int, isTTY bool) ([]shared.Environment, error)
}

type ListOptions struct {
	BaseRepo              func() (ghrepo.Interface, error)
	IO                    *iostreams.IOStreams
	Exporter              cmdutil.Exporter
	Browser               browser.Browser
	EnvironmentListClient EnvironmentListClient

	Limit int
	Web   bool
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List GitHub deployment environments",
		Example: heredoc.Doc(`
			# List environments for the current repository
			$ gh env list
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}
			opts.EnvironmentListClient = &EnvironmentLister{HTTPClient: httpClient}

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of environments to fetch")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open environments in the browser")

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.EnvironmentFields)

	return cmd
}

func listRun(opts *ListOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.Web {
		envListUrl := ghrepo.GenerateRepoURL(baseRepo, "settings/environments")

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(envListUrl))
		}

		return opts.Browser.Browse(envListUrl)
	}

	opts.IO.StartProgressIndicator()
	environments, err := opts.EnvironmentListClient.List(baseRepo, opts.Limit, opts.IO.IsStdoutTTY())
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("%s Failed to get environments: %w", opts.IO.ColorScheme().FailureIcon(), err)
	}

	if len(environments) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError(fmt.Sprintf("No environments found in %s", ghrepo.FullName(baseRepo)))
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.Out, "Failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, environments)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\nShowing %d of %s in %s\n\n", len(environments), text.Pluralize(len(environments), "environment"), ghrepo.FullName(baseRepo))
		tp := tableprinter.New(opts.IO, tableprinter.WithHeader("NAME", "PROTECTION RULES", "SECRETS", "VARIABLES"))
		for _, environment := range environments {
			tp.AddField(environment.Name)
			tp.AddField(fmt.Sprintf("%d", len(environment.ProtectionRules)))
			tp.AddField(fmt.Sprintf("%d", environment.SecretsTotalCount))
			tp.AddField(fmt.Sprintf("%d", environment.VariablesTotalCount))
			tp.EndRow()
		}
		return tp.Render()
	} else {
		for _, environment := range environments {
			fmt.Fprintf(opts.IO.Out, "%s\n", environment.Name)
		}
	}

	return nil
}
