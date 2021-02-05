package edit

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	shared "github.com/cli/cli/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	DetermineEditor    func() (string, error)
	FieldsToEditSurvey func(*prShared.Editable) error
	EditFieldsSurvey   func(*prShared.Editable, string) error
	FetchOptions       func(*api.Client, ghrepo.Interface, *prShared.Editable) error

	SelectorArg string
	Interactive bool

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
	}

	cmd := &cobra.Command{
		Use:   "edit {<number> | <url>}",
		Short: "Edit an issue",
		Example: heredoc.Doc(`
			$ gh issue edit 23 --title "I found a bug" --body "Nothing works"
			$ gh issue edit 23 --label "bug,help wanted"
			$ gh issue edit 23 --label bug --label "help wanted"
			$ gh issue edit 23 --assignee monalisa,hubot
			$ gh issue edit 23 --assignee @me
			$ gh issue edit 23 --project "Roadmap"
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
				return &cmdutil.FlagError{Err: errors.New("--tile, --body, --assignee, --label, --project, or --milestone required when not running interactively")}
			}

			if runF != nil {
				return runF(opts)
			}

			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Editable.Title, "title", "t", "", "Revise the issue title.")
	cmd.Flags().StringVarP(&opts.Editable.Body, "body", "b", "", "Revise the issue body.")
	cmd.Flags().StringSliceVarP(&opts.Editable.Assignees, "assignee", "a", nil, "Set assigned people by their `login`. Use \"@me\" to self-assign.")
	cmd.Flags().StringSliceVarP(&opts.Editable.Labels, "label", "l", nil, "Set the issue labels by `name`")
	cmd.Flags().StringSliceVarP(&opts.Editable.Projects, "project", "p", nil, "Set the projects the issue belongs to by `name`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone, "milestone", "m", "", "Set the milestone the issue belongs to by `name`")

	return cmd
}

func editRun(opts *EditOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	issue, repo, err := shared.IssueFromArg(apiClient, opts.BaseRepo, opts.SelectorArg)
	if err != nil {
		return err
	}

	editable := opts.Editable
	editable.TitleDefault = issue.Title
	editable.BodyDefault = issue.Body
	editable.AssigneesDefault = issue.Assignees
	editable.LabelsDefault = issue.Labels
	editable.ProjectsDefault = issue.ProjectCards
	editable.MilestoneDefault = issue.Milestone

	if opts.Interactive {
		err = opts.FieldsToEditSurvey(&editable)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = opts.FetchOptions(apiClient, repo, &editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.Interactive {
		editorCommand, err := opts.DetermineEditor()
		if err != nil {
			return err
		}
		err = opts.EditFieldsSurvey(&editable, editorCommand)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = updateIssue(apiClient, repo, issue.ID, editable)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, issue.URL)

	return nil
}

func updateIssue(client *api.Client, repo ghrepo.Interface, id string, options prShared.Editable) error {
	var err error
	params := githubv4.UpdateIssueInput{
		ID:    id,
		Title: options.TitleParam(),
		Body:  options.BodyParam(),
	}
	params.AssigneeIDs, err = options.AssigneesParam(client, repo)
	if err != nil {
		return err
	}
	params.LabelIDs, err = options.LabelsParam()
	if err != nil {
		return err
	}
	params.ProjectIDs, err = options.ProjectsParam()
	if err != nil {
		return err
	}
	params.MilestoneID, err = options.MilestoneParam()
	if err != nil {
		return err
	}
	return api.IssueUpdate(client, repo, params)
}
