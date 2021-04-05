package view

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	runShared "github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    cmdutil.Browser

	Selector string
	Ref      string
	Web      bool
	Prompt   bool
	Raw      bool
	YAML     bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:    "view [<workflow-id> | <workflow name> | <file name>]",
		Short:  "View the summary of a workflow",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		Example: heredoc.Doc(`
		  # Interactively select a workflow to view
		  $ gh workflow view

		  # View a specific workflow
		  $ gh workflow view 0451
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.Raw = !opts.IO.IsStdoutTTY()

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("workflow argument required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if !opts.YAML && opts.Ref != "" {
				return &cmdutil.FlagError{Err: errors.New("`--yaml` required when specifying `--ref`")}
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open workflow in the browser")
	cmd.Flags().BoolVarP(&opts.YAML, "yaml", "y", false, "View the workflow yaml file")
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to view")

	return cmd
}

func runView(opts *ViewOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	var workflow *shared.Workflow
	states := []shared.WorkflowState{shared.Active}
	workflow, err = shared.ResolveWorkflow(opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		return err
	}

	if opts.Web {
		var url string
		if opts.YAML {
			ref := opts.Ref
			if ref == "" {
				opts.IO.StartProgressIndicator()
				ref, err = api.RepoDefaultBranch(client, repo)
				opts.IO.StopProgressIndicator()
				if err != nil {
					return err
				}
			}
			url = ghrepo.GenerateRepoURL(repo, "blob/%s/%s", ref, workflow.Path)
		} else {
			url = ghrepo.GenerateRepoURL(repo, "actions/workflows/%s", workflow.Base())
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}

	if opts.YAML {
		err = viewWorkflowContent(opts, client, workflow)
	} else {
		err = viewWorkflowInfo(opts, client, workflow)
	}
	if err != nil {
		return err
	}

	return nil
}

func viewWorkflowContent(opts *ViewOptions, client *api.Client, workflow *shared.Workflow) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	opts.IO.StartProgressIndicator()
	yaml, err := getWorkflowContent(client, repo, opts.Ref, workflow)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if s, ok := err.(api.HTTPError); ok && s.StatusCode == 404 {
			if opts.Ref != "" {
				return fmt.Errorf("could not find workflow file %s on %s, try specifying a different ref", workflow.Base(), opts.Ref)
			}
			return fmt.Errorf("could not find workflow file %s, try specifying a branch or tag using `--ref`", workflow.Base())
		}
		return fmt.Errorf("could not get workflow file content: %w", err)
	}

	theme := opts.IO.DetectTerminalTheme()
	markdownStyle := markdown.GetStyle(theme)
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	if !opts.Raw {
		cs := opts.IO.ColorScheme()
		out := opts.IO.Out

		fileName := workflow.Base()
		fmt.Fprintf(out, "%s - %s\n", cs.Bold(workflow.Name), cs.Gray(fileName))
		fmt.Fprintf(out, "ID: %s", cs.Cyanf("%d", workflow.ID))

		codeBlock := fmt.Sprintf("```yaml\n%s\n```", yaml)
		rendered, err := markdown.RenderWithOpts(codeBlock, markdownStyle,
			markdown.RenderOpts{
				markdown.WithoutIndentation(),
				markdown.WithoutWrap(),
			})
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(opts.IO.Out, rendered)
		return err
	}

	if _, err := fmt.Fprint(opts.IO.Out, yaml); err != nil {
		return err
	}

	if !strings.HasSuffix(yaml, "\n") {
		_, err := fmt.Fprint(opts.IO.Out, "\n")
		return err
	}

	return nil
}

func viewWorkflowInfo(opts *ViewOptions, client *api.Client, workflow *shared.Workflow) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	opts.IO.StartProgressIndicator()
	wr, err := getWorkflowRuns(client, repo, workflow)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to get runs: %w", err)
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()
	tp := utils.NewTablePrinter(opts.IO)

	// Header
	filename := workflow.Base()
	fmt.Fprintf(out, "%s - %s\n", cs.Bold(workflow.Name), cs.Cyan(filename))
	fmt.Fprintf(out, "ID: %s\n\n", cs.Cyanf("%d", workflow.ID))

	// Runs
	fmt.Fprintf(out, "Total runs %d\n", wr.Total)

	if wr.Total != 0 {
		fmt.Fprintln(out, "Recent runs")
	}

	for _, run := range wr.Runs {
		if opts.Raw {
			tp.AddField(string(run.Status), nil, nil)
			tp.AddField(string(run.Conclusion), nil, nil)
		} else {
			symbol, symbolColor := runShared.Symbol(cs, run.Status, run.Conclusion)
			tp.AddField(symbol, nil, symbolColor)
		}

		tp.AddField(run.CommitMsg(), nil, cs.Bold)

		tp.AddField(run.Name, nil, nil)
		tp.AddField(run.HeadBranch, nil, cs.Bold)
		tp.AddField(string(run.Event), nil, nil)

		if opts.Raw {
			elapsed := run.UpdatedAt.Sub(run.CreatedAt)
			if elapsed < 0 {
				elapsed = 0
			}
			tp.AddField(elapsed.String(), nil, nil)
		}

		tp.AddField(fmt.Sprintf("%d", run.ID), nil, cs.Cyan)

		tp.EndRow()
	}

	err = tp.Render()
	if err != nil {
		return err
	}

	fmt.Fprintln(out)

	// Footer
	if wr.Total != 0 {
		fmt.Fprintf(out, "To see more runs for this workflow, try: gh run list --workflow %s\n", filename)
	}
	fmt.Fprintf(out, "To see the YAML for this workflow, try: gh workflow view %s --yaml\n", filename)
	return nil
}
