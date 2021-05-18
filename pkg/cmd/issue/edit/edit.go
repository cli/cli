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

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "edit {<number> | <url>}",
		Short: "Edit an issue",
		Example: heredoc.Doc(`
			$ gh issue edit 23 --title "I found a bug" --body "Nothing works"
			$ gh issue edit 23 --add-label "bug,help wanted" --remove-label "core"
			$ gh issue edit 23 --add-assignee @me --remove-assignee monalisa,hubot
			$ gh issue edit 23 --add-project "Roadmap" --remove-project v1,v2
			$ gh issue edit 23 --milestone "Version 1"
			$ gh issue edit 23 --body-file body.txt
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.SelectorArg = args[0]

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
				return &cmdutil.FlagError{Err: errors.New("field to edit flag required when not running interactively")}
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
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Add, "add-assignee", nil, "Add assigned users by their `login`. Use \"@me\" to assign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Assignees.Remove, "remove-assignee", nil, "Remove assigned users by their `login`. Use \"@me\" to unassign yourself.")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Add, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Labels.Remove, "remove-label", nil, "Remove labels by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Add, "add-project", nil, "Add the issue to projects by `name`")
	cmd.Flags().StringSliceVar(&opts.Editable.Projects.Remove, "remove-project", nil, "Remove the issue from projects by `name`")
	cmd.Flags().StringVarP(&opts.Editable.Milestone.Value, "milestone", "m", "", "Edit the milestone the issue belongs to by `name`")

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
	editable.Title.Default = issue.Title
	editable.Body.Default = issue.Body
	editable.Assignees.Default = issue.Assignees.Logins()
	editable.Labels.Default = issue.Labels.Names()
	editable.Projects.Default = issue.ProjectCards.ProjectNames()
	if issue.Milestone != nil {
		editable.Milestone.Default = issue.Milestone.Title
	}

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
		Title: ghString(options.TitleValue()),
		Body:  ghString(options.BodyValue()),
	}
	assigneeIds, err := options.AssigneeIds(client, repo)
	if err != nil {
		return err
	}
	params.AssigneeIDs = ghIds(assigneeIds)
	labelIds, err := options.LabelIds()
	if err != nil {
		return err
	}
	params.LabelIDs = ghIds(labelIds)
	projectIds, err := options.ProjectIds()
	if err != nil {
		return err
	}
	params.ProjectIDs = ghIds(projectIds)
	milestoneId, err := options.MilestoneId()
	if err != nil {
		return err
	}
	params.MilestoneID = ghId(milestoneId)
	return api.IssueUpdate(client, repo, params)
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
