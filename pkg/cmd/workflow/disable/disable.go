package disable

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DisableOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	Selector     string
	SelectorArgs []string
	Prompt       bool
	Multi        bool
}

type iprompter interface {
	Select(string, string, []string) (int, error)
}

func NewCmdDisable(f *cmdutil.Factory, runF func(*DisableOptions) error) *cobra.Command {
	opts := &DisableOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "disable [<workflow-id> | <workflow-name>]",
		Short: "Disable a workflow",
		Long:  "Disable a workflow, preventing it from running or showing up when listing workflows.\n\nUse the --multi flag to disable multiple workflows at once.",
		Example: "  # Disable a single workflow\n" +
			"  $ gh workflow disable 123\n" +
			"  $ gh workflow disable workflow-name\n\n" +
			"  # Disable multiple workflows\n" +
			"  $ gh workflow disable --multi workflow1 workflow2 123",
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			// Validate arguments based on mode
			if !opts.Multi && len(args) > 1 {
				return fmt.Errorf("too many arguments, use --multi flag to disable multiple workflows")
			}

			// Process arguments
			if len(args) > 0 {
				if opts.Multi {
					opts.SelectorArgs = args
				} else {
					opts.Selector = args[0]
				}
			} else if opts.Multi {
				// Multi flag requires at least one argument
				return cmdutil.FlagErrorf("at least one workflow ID or name required with --multi flag")
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("workflow ID or name required when not running interactively")
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runDisable(opts)
		},
	}

	// Add flags
	cmd.Flags().BoolVar(&opts.Multi, "multi", false, "Disable multiple workflows at once")

	return cmd
}

// runDisable handles the workflow disabling process
func runDisable(opts *DisableOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	// Get repository
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	// Handle multi-mode or single workflow mode
	if opts.Multi {
		return disableMultipleWorkflows(client, repo, opts)
	}

	// Handle single workflow mode
	workflow, err := resolveAndDisableWorkflow(client, repo, opts, opts.Selector, opts.Prompt)
	if err != nil {
		var fae shared.FilteredAllError
		if errors.As(err, &fae) {
			return errors.New("there are no enabled workflows to disable")
		}
		return err
	}

	printDisabledMessage(opts.IO, workflow.Name)
	return nil
}

// disableMultipleWorkflows handles disabling multiple workflows with error collection
func disableMultipleWorkflows(client *api.Client, repo ghrepo.Interface, opts *DisableOptions) error {
	var errs []error
	var successCount int

	resolvedSelectors := make(map[string]int64) // Maps selectors to workflow IDs
	disabledWorkflowIDs := make(map[int64]bool) // Tracks already disabled workflow IDs

	for _, selector := range opts.SelectorArgs {
		var workflow *shared.Workflow
		var err error

		// Check if we've already resolved this selector
		if workflowID, exists := resolvedSelectors[selector]; exists {
			// Check if we've already disabled this workflow
			if disabledWorkflowIDs[workflowID] {
				// Workflow already disabled, skip silently
				continue
			}

			// Reuse the already resolved workflow ID to avoid prompting again
			path := fmt.Sprintf("repos/%s/actions/workflows/%d/disable", ghrepo.FullName(repo), workflowID)
			err = client.REST(repo.RepoHost(), "PUT", path, nil, nil)
			if err != nil {
				errs = append(errs, fmt.Errorf("workflow '%s': failed to disable workflow: %w", selector, err))
				continue
			}

			disabledWorkflowIDs[workflowID] = true
			successCount++
			continue
		}

		// Resolve and disable workflow normally
		workflow, err = resolveAndDisableWorkflow(client, repo, opts, selector, false)
		if err != nil {
			var fae shared.FilteredAllError
			if errors.As(err, &fae) {
				errs = append(errs, fmt.Errorf("workflow '%s': there are no enabled workflows to disable", selector))
			} else {
				errs = append(errs, fmt.Errorf("workflow '%s': %w", selector, err))
			}
			continue
		}

		// Store the workflow ID for this selector to avoid prompting again
		resolvedSelectors[selector] = workflow.ID
		disabledWorkflowIDs[workflow.ID] = true

		successCount++
		printDisabledMessage(opts.IO, workflow.Name)
	}

	if len(errs) > 0 {
		if successCount > 0 {
			// Some workflows were successfully disabled, others failed
			fmt.Fprintf(opts.IO.Out, "%s: Successfully disabled %d out of %d workflows\n",
				opts.IO.ColorScheme().WarningIcon(),
				successCount,
				len(opts.SelectorArgs))
		}
		return errors.Join(errs...)
	} else if opts.Multi && opts.IO.CanPrompt() {
		// Add this block for complete success message
		fmt.Fprintf(opts.IO.Out, "%s Successfully disabled all %d workflows\n",
			opts.IO.ColorScheme().SuccessIcon(),
			successCount)
	}

	return nil
}

// resolveAndDisableWorkflow handles resolving and disabling a single workflow
func resolveAndDisableWorkflow(client *api.Client, repo ghrepo.Interface, opts *DisableOptions, selector string, usePrompt bool) (*shared.Workflow, error) {
	states := []shared.WorkflowState{shared.Active}
	workflow, err := shared.ResolveWorkflow(
		opts.Prompter, opts.IO, client, repo, usePrompt, selector, states)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("repos/%s/actions/workflows/%d/disable", ghrepo.FullName(repo), workflow.ID)
	err = client.REST(repo.RepoHost(), "PUT", path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to disable workflow: %w", err)
	}

	return workflow, nil
}

// printDisabledMessage prints a success message for a disabled workflow
func printDisabledMessage(io *iostreams.IOStreams, workflowName string) {
	if io.CanPrompt() {
		cs := io.ColorScheme()
		fmt.Fprintf(io.Out, "%s Disabled %s\n", cs.SuccessIconWithColor(cs.Red), cs.Bold(workflowName))
	}
}
