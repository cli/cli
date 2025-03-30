package edit

import (
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	shared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   prShared.EditPrompter

	DetermineEditor    func() (string, error)
	FieldsToEditSurvey func(prShared.EditPrompter, *prShared.Editable) error
	EditFieldsSurvey   func(prShared.EditPrompter, *prShared.Editable, string) error
	FetchOptions       func(*api.Client, ghrepo.Interface, *prShared.Editable) error

	SelectorArgs []string
	Interactive  bool

	prShared.Editable
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:                 f.IOStreams,
		HttpClient:         f.HttpClient,
		DetermineEditor:    func() (string, error) { return cmdutil.DetermineEditor(f.Config) },
		FieldsToEditSurvey: prShared.FieldsToEditSurvey,
		EditFieldsSurvey:   prShared.EditFieldsSurvey,
		FetchOptions:       prShared.FetchOptions,
		Prompter:           f.Prompter,
	}

	var bodyFile string
	var removeMilestone bool

	cmd := &cobra.Command{
		Use:   "edit {<numbers> | <urls>}",
		Short: "Edit issues",
		Long: heredoc.Docf(`
			Edit one or more issues within the same repository.

			Editing issues' projects requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh issue edit 23 --title "I found a bug" --body "Nothing works"
			$ gh issue edit 23 --add-label "bug,help wanted" --remove-label "core"
			$ gh issue edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
			$ gh issue edit 23 --add-project "Roadmap" --remove-project v1,v2
			$ gh issue edit 23 --milestone "Version 1"
			$ gh issue edit 23 --remove-milestone
			$ gh issue edit 23 --body-file body.txt
			$ gh issue edit 23 34 --add-label "help wanted"
		`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.SelectorArgs = args

			flags := cmd.Flags()

			bodyProvided := flags.Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}
			if bodyProvided || bodyFileProvided {
				opts.Editable.Body.Edited = true
				if bodyFileProvided {
					b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
					if err != nil {
						return err
					}
					opts.Editable.Body.Value = string(b)
				}
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--milestone` or `--remove-milestone`",
				flags.Changed("milestone"),
				removeMilestone,
			); err != nil {
				return err
			}

			if flags.Changed("title") {
				opts.Editable.Title.Edited = true
			}
			if flags.Changed("add-assignee") || flags.Changed("remove-assignee") {
				opts.Editable.Assignees.Edited = true
			}
			if flags.Changed("add-label") || flags.Changed("remove-label") {
				opts.Editable.Labels.Edited = true
			}
			if flags.Changed("add-project") || flags.Changed("remove-project") {
				opts.Editable.Projects.Edited = true
			}
			if flags.Changed("milestone") || removeMilestone {
				opts.Editable.Milestone.Edited = true

				// Note that when `--remove-milestone` is provided, the value of
				// `opts.Editable.Milestone.Value` will automatically be empty,
				// which results in milestone association removal. For reference,
				// see the `Editable.MilestoneId` method.
			}

			if !opts.Editable.Dirty() {
				opts.Interactive = true
			}

			if opts.Interactive && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("field to edit flag required when not running interactively")
			}

			if opts.Interactive && len(opts.SelectorArgs) > 1 {
				return cmdutil.FlagErrorf("multiple issues cannot be edited interactively")
			}

			if runF != nil {
				return runF(opts)
			}

			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Editable.Title.Value, "title", "t", "", "Set the new title.")
	cmd.Flags().StringVarP(&opts.Editable.Body.Value, "body", "b", "", "Set the new body.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Add, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Remove, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Add, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Remove, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Add, "add-project", nil, "Add the issue to projects by `title`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Remove, "remove-project", nil, "Remove the issue from projects by `title`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone.Value, "milestone", "m", "", "Edit the milestone the issue belongs to by `name`")
	cmd.Flags().BoolVar(&removeMilestone, "remove-milestone", false, "Remove the milestone association from the issue")

	return cmd
}

func editRun(opts *EditOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	// Prompt the user which fields they'd like to edit.
	editable := opts.Editable
	if opts.Interactive {
		err = opts.FieldsToEditSurvey(opts.Prompter, &editable)
		if err != nil {
			return err
		}
	}

	lookupFields := []string{"id", "number", "title", "body", "url"}
	if editable.Assignees.Edited {
		lookupFields = append(lookupFields, "assignees")
	}
	if editable.Labels.Edited {
		lookupFields = append(lookupFields, "labels")
	}
	if editable.Projects.Edited {
		lookupFields = append(lookupFields, "projectCards")
		lookupFields = append(lookupFields, "projectItems")
	}
	if editable.Milestone.Edited {
		lookupFields = append(lookupFields, "milestone")
	}

	// Get all specified issues and make sure they are within the same repo.
	issues, repo, err := shared.IssuesFromArgsWithFields(httpClient, opts.BaseRepo, opts.SelectorArgs, lookupFields)
	if err != nil {
		return err
	}

	// Fetch editable shared fields once for all issues.
	apiClient := api.NewClientFromHTTP(httpClient)
	opts.IO.StartProgressIndicatorWithLabel("Fetching repository information")
	err = opts.FetchOptions(apiClient, repo, &editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	// Update all issues in parallel.
	editedIssueChan := make(chan string, len(issues))
	failedIssueChan := make(chan string, len(issues))
	g := sync.WaitGroup{}

	// Only show progress if we will not prompt below or the survey will break up the progress indicator.
	if !opts.Interactive {
		opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Updating %d issues", len(issues)))
	}

	for _, issue := range issues {
		// Copy variables to capture in the go routine below.
		editable := editable.Clone()

		editable.Title.Default = issue.Title
		editable.Body.Default = issue.Body
		editable.Assignees.Default = issue.Assignees.Logins()
		editable.Labels.Default = issue.Labels.Names()
		editable.Projects.Default = append(issue.ProjectCards.ProjectNames(), issue.ProjectItems.ProjectTitles()...)
		projectItems := map[string]string{}
		for _, n := range issue.ProjectItems.Nodes {
			projectItems[n.Project.ID] = n.ID
		}
		editable.Projects.ProjectItems = projectItems
		if issue.Milestone != nil {
			editable.Milestone.Default = issue.Milestone.Title
		}

		// Allow interactive prompts for one issue; failed earlier if multiple issues specified.
		if opts.Interactive {
			editorCommand, err := opts.DetermineEditor()
			if err != nil {
				return err
			}
			err = opts.EditFieldsSurvey(opts.Prompter, &editable, editorCommand)
			if err != nil {
				return err
			}
		}

		g.Add(1)
		go func(issue *api.Issue) {
			defer g.Done()

			err := prShared.UpdateIssue(httpClient, repo, issue.ID, issue.IsPullRequest(), editable)
			if err != nil {
				failedIssueChan <- fmt.Sprintf("failed to update %s: %s", issue.URL, err)
				return
			}

			editedIssueChan <- issue.URL
		}(issue)
	}

	g.Wait()
	close(editedIssueChan)
	close(failedIssueChan)

	// Does nothing if progress was not started above.
	opts.IO.StopProgressIndicator()

	// Print a sorted list of successfully edited issue URLs to stdout.
	editedIssueURLs := make([]string, 0, len(issues))
	for editedIssueURL := range editedIssueChan {
		editedIssueURLs = append(editedIssueURLs, editedIssueURL)
	}

	sort.Strings(editedIssueURLs)
	for _, editedIssueURL := range editedIssueURLs {
		fmt.Fprintln(opts.IO.Out, editedIssueURL)
	}

	// Print a sorted list of failures to stderr.
	failedIssueErrors := make([]string, 0, len(issues))
	for failedIssueError := range failedIssueChan {
		failedIssueErrors = append(failedIssueErrors, failedIssueError)
	}

	sort.Strings(failedIssueErrors)
	for _, failedIssueError := range failedIssueErrors {
		fmt.Fprintln(opts.IO.ErrOut, failedIssueError)
	}

	if len(failedIssueErrors) > 0 {
		return fmt.Errorf("failed to update %s", text.Pluralize(len(failedIssueErrors), "issue"))
	}

	return nil
}
