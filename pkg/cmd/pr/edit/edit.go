package edit

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
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
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

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
		Remotes:         f.Remotes,
		Branch:          f.Branch,
		Surveyor:        surveyor{},
		Fetcher:         fetcher{},
		EditorRetriever: editorRetriever{config: f.Config},
	}

	cmd := &cobra.Command{
		Use:   "edit {<number> | <url>}",
		Short: "Edit a pull request",
		Example: heredoc.Doc(`
			$ gh pr edit 23 --title "I found a bug" --body "Nothing works"
			$ gh pr edit 23 --label "bug,help wanted"
			$ gh pr edit 23 --label bug --label "help wanted"
			$ gh pr edit 23 --reviewer monalisa,hubot  --reviewer myorg/team-name
			$ gh pr edit 23 --assignee monalisa,hubot
			$ gh pr edit 23 --assignee @me
			$ gh pr edit 23 --project "Roadmap"
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.SelectorArg = args[0]

			flags := cmd.Flags()
			if flags.Changed("title") {
				opts.Editable.TitleEdited = true
			}
			if flags.Changed("body") {
				opts.Editable.BodyEdited = true
			}
			if flags.Changed("reviewer") {
				opts.Editable.ReviewersEdited = true
			}
			if flags.Changed("assignee") {
				opts.Editable.AssigneesEdited = true
			}
			if flags.Changed("label") {
				opts.Editable.LabelsEdited = true
			}
			if flags.Changed("project") {
				opts.Editable.ProjectsEdited = true
			}
			if flags.Changed("milestone") {
				opts.Editable.MilestoneEdited = true
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

	cmd.Flags().StringVarP(&opts.Editable.Title, "title", "t", "", "Revise the pr title.")
	cmd.Flags().StringVarP(&opts.Editable.Body, "body", "b", "", "Revise the pr body.")
	cmd.Flags().StringSliceVarP(&opts.Editable.Reviewers, "reviewer", "r", nil, "Request reviews from people or teams by their `handle`")
	cmd.Flags().StringSliceVarP(&opts.Editable.Assignees, "assignee", "a", nil, "Set assigned people by their `login`. Use \"@me\" to self-assign.")
	cmd.Flags().StringSliceVarP(&opts.Editable.Labels, "label", "l", nil, "Set the pr labels by `name`")
	cmd.Flags().StringSliceVarP(&opts.Editable.Projects, "project", "p", nil, "Set the projects the pr belongs to by `name`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone, "milestone", "m", "", "Set the milestone the pr belongs to by `name`")

	return cmd
}

func editRun(opts *EditOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, repo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	editable := opts.Editable
	editable.ReviewersAllowed = true
	editable.TitleDefault = pr.Title
	editable.BodyDefault = pr.Body
	editable.ReviewersDefault = pr.ReviewRequests
	editable.AssigneesDefault = pr.Assignees
	editable.LabelsDefault = pr.Labels
	editable.ProjectsDefault = pr.ProjectCards
	editable.MilestoneDefault = pr.Milestone

	if opts.Interactive {
		err = opts.Surveyor.FieldsToEdit(&editable)
		if err != nil {
			return err
		}
	}

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
		Title:         editable.TitleParam(),
		Body:          editable.BodyParam(),
	}
	params.AssigneeIDs, err = editable.AssigneesParam(client, repo)
	if err != nil {
		return err
	}
	params.LabelIDs, err = editable.LabelsParam()
	if err != nil {
		return err
	}
	params.ProjectIDs, err = editable.ProjectsParam()
	if err != nil {
		return err
	}
	params.MilestoneID, err = editable.MilestoneParam()
	if err != nil {
		return err
	}
	err = api.UpdatePullRequest(client, repo, params)
	if err != nil {
		return err
	}
	return updatePullRequestReviews(client, repo, id, editable)
}

func updatePullRequestReviews(client *api.Client, repo ghrepo.Interface, id string, editable shared.Editable) error {
	if !editable.ReviewersEdited {
		return nil
	}
	userIds, teamIds, err := editable.ReviewersParams()
	if err != nil {
		return err
	}
	union := githubv4.Boolean(false)
	reviewsRequestParams := githubv4.RequestReviewsInput{
		PullRequestID: id,
		Union:         &union,
		UserIDs:       userIds,
		TeamIDs:       teamIds,
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
