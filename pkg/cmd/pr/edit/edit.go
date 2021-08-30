package edit

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	shared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder          shared.PRFinder
	Surveyor        Surveyor
	Fetcher         EditableOptionsFetcher
	EditorRetriever EditorRetriever

	SelectorArg string
	Interactive bool

	shared.Editable
}

func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:              f.IOStreams,
		HttpClient:      f.HttpClient,
		Surveyor:        surveyor{},
		Fetcher:         fetcher{},
		EditorRetriever: editorRetriever{config: f.Config},
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "edit [<number> | <url> | <branch>]",
		Short: "Edit a pull request",
		Long: heredoc.Doc(`
			Edit a pull request.

			Without an argument, the pull request that belongs to the current branch
			is selected.
		`),
		Example: heredoc.Doc(`
			$ gh pr edit 23 --title "I found a bug" --body "Nothing works"
			$ gh pr edit 23 --add-label "bug,help wanted" --remove-label "core"
			$ gh pr edit 23 --add-reviewer monalisa,hubot  --remove-reviewer myorg/team-name
			$ gh pr edit 23 --add-assignee "@me" --remove-assignee monalisa,hubot
			$ gh pr edit 23 --add-project "Roadmap" --remove-project v1,v2
			$ gh pr edit 23 --milestone "Version 1"
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
			if flags.Changed("milestone") {
				opts.Editable.Milestone.Edited = true
			}

			if !opts.Editable.Dirty() {
				opts.Interactive = true
			}

			if opts.Interactive && !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("--tile, --body, --reviewer, --assignee, --label, --project, or --milestone required when not running interactively")}
			}

			if runF != nil {
				return runF(opts)
			}

			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Editable.Title.Value, "title", "t", "", "Set the new title.")
	cmd.Flags().StringVarP(&opts.Editable.Body.Value, "body", "b", "", "Set the new body.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file`")
	cmd.Flags().StringVarP(&opts.Editable.Base.Value, "base", "B", "", "Change the base `branch` for this pull request")
	cmd.Flags().StringSliceVar(&opts.Editable.Reviewers.Add, "add-reviewer", nil, "Add reviewers by their `login`.")
	cmd.Flags().StringSliceVar(&opts.Editable.Reviewers.Remove, "remove-reviewer", nil, "Remove reviewers by their `login`.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Add, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Remove, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Add, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Remove, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Add, "add-project", nil, "Add the pull request to projects by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Remove, "remove-project", nil, "Remove the pull request from projects by `name`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone.Value, "milestone", "m", "", "Edit the milestone the pull request belongs to by `name`")

	return cmd
}

func editRun(opts *EditOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "url", "title", "body", "baseRefName", "reviewRequests", "assignees", "labels", "projectCards", "milestone"},
	}
	pr, repo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	editable := opts.Editable
	editable.Reviewers.Allowed = true
	editable.Title.Default = pr.Title
	editable.Body.Default = pr.Body
	editable.Base.Default = pr.BaseRefName
	editable.Reviewers.Default = pr.ReviewRequests.Logins()
	editable.Assignees.Default = pr.Assignees.Logins()
	editable.Labels.Default = pr.Labels.Names()
	editable.Projects.Default = pr.ProjectCards.ProjectNames()
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
	err = updatePullRequest(apiClient, repo, pr.ID, editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, pr.URL)

	return nil
}

func updatePullRequest(client *api.Client, repo ghrepo.Interface, id string, editable shared.Editable) error {
	var err error
	params := githubv4.UpdatePullRequestInput{
		PullRequestID: id,
		Title:         ghString(editable.TitleValue()),
		Body:          ghString(editable.BodyValue()),
	}
	assigneeIds, err := editable.AssigneeIds(client, repo)
	if err != nil {
		return err
	}
	params.AssigneeIDs = ghIds(assigneeIds)
	labelIds, err := editable.LabelIds()
	if err != nil {
		return err
	}
	params.LabelIDs = ghIds(labelIds)
	projectIds, err := editable.ProjectIds()
	if err != nil {
		return err
	}
	params.ProjectIDs = ghIds(projectIds)
	milestoneId, err := editable.MilestoneId()
	if err != nil {
		return err
	}
	params.MilestoneID = ghId(milestoneId)
	if editable.Base.Edited {
		params.BaseRefName = ghString(&editable.Base.Value)
	}
	err = api.UpdatePullRequest(client, repo, params)
	if err != nil {
		return err
	}
	return updatePullRequestReviews(client, repo, id, editable)
}

func updatePullRequestReviews(client *api.Client, repo ghrepo.Interface, id string, editable shared.Editable) error {
	if !editable.Reviewers.Edited {
		return nil
	}
	userIds, teamIds, err := editable.ReviewerIds()
	if err != nil {
		return err
	}
	union := githubv4.Boolean(false)
	reviewsRequestParams := githubv4.RequestReviewsInput{
		PullRequestID: id,
		Union:         &union,
		UserIDs:       ghIds(userIds),
		TeamIDs:       ghIds(teamIds),
	}
	return api.UpdatePullRequestReviews(client, repo, reviewsRequestParams)
}

type Surveyor interface {
	FieldsToEdit(*shared.Editable) error
	EditFields(*shared.Editable, string) error
}

type surveyor struct{}

func (s surveyor) FieldsToEdit(editable *shared.Editable) error {
	return shared.FieldsToEditSurvey(editable)
}

func (s surveyor) EditFields(editable *shared.Editable, editorCmd string) error {
	return shared.EditFieldsSurvey(editable, editorCmd)
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
	config func() (config.Config, error)
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

func ghId(s *string) *githubv4.ID {
	if s == nil {
		return nil
	}
	if *s == "" {
		r := githubv4.ID(nil)
		return &r
	}
	r := githubv4.ID(*s)
	return &r
}

func ghString(s *string) *githubv4.String {
	if s == nil {
		return nil
	}
	r := githubv4.String(*s)
	return &r
}
