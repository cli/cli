package edit

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder          shared.PRFinder
	Surveyor        Surveyor
	Fetcher         EditableOptionsFetcher
	EditorRetriever EditorRetriever
	Prompter        shared.EditPrompter

	SelectorArg string
	Interactive bool

	shared.Editable
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:              f.IOStreams,
		HttpClient:      f.HttpClient,
		Surveyor:        surveyor{P: f.Prompter},
		Fetcher:         fetcher{},
		EditorRetriever: editorRetriever{config: f.Config},
		Prompter:        f.Prompter,
	}

	var bodyFile string
	var removeMilestone bool

	cmd := &cobra.Command{
		Use:   "edit [<number> | <url> | <branch>]",
		Short: "Edit a pull request",
		Long: heredoc.Docf(`
			Edit a pull request.

			Without an argument, the pull request that belongs to the current branch
			is selected.

			Editing a pull request's projects requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh pr edit 23 --title "I found a bug" --body "Nothing works"
			$ gh pr edit 23 --add-label "bug,help wanted" --remove-label "core"
			$ gh pr edit 23 --add-reviewer monalisa,hubot  --remove-reviewer myorg/team-name
			$ gh pr edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
			$ gh pr edit 23 --add-project "Roadmap" --remove-project v1,v2
			$ gh pr edit 23 --milestone "Version 1"
			$ gh pr edit 23 --remove-milestone
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

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
			if flags.Changed("body") {
				opts.Editable.Body.Edited = true
			}
			if flags.Changed("base") {
				opts.Editable.Base.Edited = true
			}
			if flags.Changed("add-reviewer") || flags.Changed("remove-reviewer") {
				opts.Editable.Reviewers.Edited = true
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
				return cmdutil.FlagErrorf("--tile, --body, --reviewer, --assignee, --label, --project, or --milestone required when not running interactively")
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
	cmd.Flags().StringVarP(&opts.Editable.Base.Value, "base", "B", "", "Change the base `branch` for this pull request")
	cmd.Flags().StringSliceVar(&opts.Editable.Reviewers.Add, "add-reviewer", nil, "Add reviewers by their `login`.")
	cmd.Flags().StringSliceVar(&opts.Editable.Reviewers.Remove, "remove-reviewer", nil, "Remove reviewers by their `login`.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Add, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Remove, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Add, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Remove, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Add, "add-project", nil, "Add the pull request to projects by `title`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Remove, "remove-project", nil, "Remove the pull request from projects by `title`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone.Value, "milestone", "m", "", "Edit the milestone the pull request belongs to by `name`")
	cmd.Flags().BoolVar(&removeMilestone, "remove-milestone", false, "Remove the milestone association from the pull request")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "base")

	for _, flagName := range []string{"add-reviewer", "remove-reviewer"} {
		_ = cmd.RegisterFlagCompletionFunc(flagName, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			baseRepo, err := f.BaseRepo()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			httpClient, err := f.HttpClient()
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			results, err := shared.RequestableReviewersForCompletion(httpClient, baseRepo)
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}
			return results, cobra.ShellCompDirectiveNoFileComp
		})
	}

	return cmd
}

func editRun(opts *EditOptions) error {
	editable := opts.Editable
	lookupFields := []string{"id", "url", "title", "body", "baseRefName"}

	if opts.Interactive {
		lookupFields = append(lookupFields, "assignees", "labels")
	}

	if opts.Interactive || editable.Reviewers.Edited && len(editable.Reviewers.Remove) != 0 {
		lookupFields = append(lookupFields, "reviewRequests")
	}

	if opts.Interactive || editable.Projects.Edited {
		lookupFields = append(lookupFields, "projectCards", "projectItems")
	}

	if opts.Interactive || editable.Milestone.Edited {
		lookupFields = append(lookupFields, "milestone")
	}

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   lookupFields,
	}
	pr, repo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	editable.Reviewers.Allowed = true
	editable.Title.Default = pr.Title
	editable.Body.Default = pr.Body
	editable.Base.Default = pr.BaseRefName
	editable.Reviewers.Default = pr.ReviewRequests.Logins()
	editable.Assignees.Default = pr.Assignees.Logins()
	editable.Labels.Default = pr.Labels.Names()
	editable.Projects.Default = append(pr.ProjectCards.ProjectNames(), pr.ProjectItems.ProjectTitles()...)
	projectItems := map[string]string{}
	for _, n := range pr.ProjectItems.Nodes {
		projectItems[n.Project.ID] = n.ID
	}
	editable.Projects.ProjectItems = projectItems
	if pr.Milestone != nil {
		editable.Milestone.Default = pr.Milestone.Title
	}

	if opts.Interactive {
		err = opts.Surveyor.FieldsToEdit(&editable)
		if err != nil {
			return err
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()
	err = opts.Fetcher.EditableOptionsFetch(apiClient, repo, &editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.Interactive {
		editorCommand, err := opts.EditorRetriever.Retrieve()
		if err != nil {
			return err
		}
		err = opts.Surveyor.EditFields(&editable, editorCommand)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = updatePullRequest(httpClient, repo, pr.ID, editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, pr.URL)

	return nil
}

func updatePullRequest(httpClient *http.Client, repo ghrepo.Interface, id string, editable shared.Editable) error {
	var wg errgroup.Group
	wg.Go(func() error {
		return shared.UpdateIssue(httpClient, repo, id, true, editable)
	})
	if editable.Reviewers.Edited {
		wg.Go(func() error {
			return updatePullRequestReviews(httpClient, repo, id, editable)
		})
	}
	return wg.Wait()
}

func updatePullRequestReviews(httpClient *http.Client, repo ghrepo.Interface, id string, editable shared.Editable) error {
	userIds, teamIds, err := editable.ReviewerIds()
	if err != nil {
		return err
	}
	if (userIds == nil || len(*userIds) == 0) &&
		(teamIds == nil || len(*teamIds) == 0) {
		return nil
	}
	union := githubv4.Boolean(false)

	if editable.Reviewers.Edited && len(editable.Reviewers.Add) != 0 && len(editable.Reviewers.Remove) == 0 {
		union = true
	}

	reviewsRequestParams := githubv4.RequestReviewsInput{
		PullRequestID: id,
		Union:         &union,
		UserIDs:       ghIds(userIds),
		TeamIDs:       ghIds(teamIds),
	}
	client := api.NewClientFromHTTP(httpClient)
	return api.UpdatePullRequestReviews(client, repo, reviewsRequestParams)
}

type Surveyor interface {
	FieldsToEdit(*shared.Editable) error
	EditFields(*shared.Editable, string) error
}

type surveyor struct {
	P shared.EditPrompter
}

func (s surveyor) FieldsToEdit(editable *shared.Editable) error {
	return shared.FieldsToEditSurvey(s.P, editable)
}

func (s surveyor) EditFields(editable *shared.Editable, editorCmd string) error {
	return shared.EditFieldsSurvey(s.P, editable, editorCmd)
}

type EditableOptionsFetcher interface {
	EditableOptionsFetch(*api.Client, ghrepo.Interface, *shared.Editable) error
}

type fetcher struct{}

func (f fetcher) EditableOptionsFetch(client *api.Client, repo ghrepo.Interface, opts *shared.Editable) error {
	return shared.FetchOptions(client, repo, opts)
}

type EditorRetriever interface {
	Retrieve() (string, error)
}

type editorRetriever struct {
	config func() (gh.Config, error)
}

func (e editorRetriever) Retrieve() (string, error) {
	return cmdutil.DetermineEditor(e.config)
}

func ghIds(s *[]string) *[]githubv4.ID {
	if s == nil {
		return nil
	}
	ids := make([]githubv4.ID, len(*s))
	for i, v := range *s {
		ids[i] = v
	}
	return &ids
}
