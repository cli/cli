package create

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	RootDirOverride string

	HasRepoOverride bool
	WebMode         bool
	RecoverFile     string

	Title       string
	Body        string
	Interactive bool

	Assignees []string
	Labels    []string
	Projects  []string
	Milestone string
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
		Example: heredoc.Doc(`
			$ gh issue create --title "I found a bug" --body "Nothing works"
			$ gh issue create --label "bug,help wanted"
			$ gh issue create --label bug --label "help wanted"
			$ gh issue create --assignee monalisa,hubot
			$ gh issue create --assignee "@me"
			$ gh issue create --project "Roadmap"
		`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			titleProvided := cmd.Flags().Changed("title")
			bodyProvided := cmd.Flags().Changed("body")
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
				bodyProvided = true
			}

			if !opts.IO.CanPrompt() && opts.RecoverFile != "" {
				return &cmdutil.FlagError{Err: errors.New("`--recover` only supported when running interactively")}
			}

			opts.Interactive = !(titleProvided && bodyProvided)

			if opts.Interactive && !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("must provide title and body when not running interactively")}
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Supply a title. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file`")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the browser to create an issue")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their `login`. Use \"@me\" to self-assign.")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVarP(&opts.Projects, "project", "p", nil, "Add the issue to projects by `name`")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the issue to a milestone by `name`")
	cmd.Flags().StringVar(&opts.RecoverFile, "recover", "", "Recover input from a failed run of create")

	return cmd
}

func createRun(opts *CreateOptions) (err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return
	}

	isTerminal := opts.IO.IsStdoutTTY()

	var milestones []string
	if opts.Milestone != "" {
		milestones = []string{opts.Milestone}
	}

	meReplacer := shared.NewMeReplacer(apiClient, baseRepo.RepoHost())
	assignees, err := meReplacer.ReplaceSlice(opts.Assignees)
	if err != nil {
		return err
	}

	tb := prShared.IssueMetadataState{
		Type:       prShared.IssueMetadata,
		Assignees:  assignees,
		Labels:     opts.Labels,
		Projects:   opts.Projects,
		Milestones: milestones,
		Title:      opts.Title,
		Body:       opts.Body,
	}

	if opts.RecoverFile != "" {
		err = prShared.FillFromJSON(opts.IO, opts.RecoverFile, &tb)
		if err != nil {
			err = fmt.Errorf("failed to recover input: %w", err)
			return
		}
	}

	tpl := shared.NewTemplateManager(httpClient, baseRepo, opts.RootDirOverride, !opts.HasRepoOverride, false)

	if opts.WebMode {
		var openURL string
		if opts.Title != "" || opts.Body != "" || tb.HasMetadata() {
			openURL, err = generatePreviewURL(apiClient, baseRepo, tb)
			if err != nil {
				return
			}
			if !utils.ValidURL(openURL) {
				err = fmt.Errorf("cannot open in browser: maximum URL length exceeded")
				return
			}
		} else if ok, _ := tpl.HasTemplates(); ok {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new/choose")
		} else {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new")
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "\nCreating issue in %s\n\n", ghrepo.FullName(baseRepo))
	}

	repo, err := api.GitHubRepo(apiClient, baseRepo)
	if err != nil {
		return
	}
	if !repo.HasIssuesEnabled {
		err = fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(baseRepo))
		return
	}

	action := prShared.SubmitAction
	templateNameForSubmit := ""
	var openURL string

	if opts.Interactive {
		var editorCommand string
		editorCommand, err = cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return
		}

		defer prShared.PreserveInput(opts.IO, &tb, &err)()

		if opts.Title == "" {
			err = prShared.TitleSurvey(&tb)
			if err != nil {
				return
			}
		}

		if opts.Body == "" {
			templateContent := ""

			if opts.RecoverFile == "" {
				var template shared.Template
				template, err = tpl.Choose()
				if err != nil {
					return
				}

				if template != nil {
					templateContent = string(template.Body())
					templateNameForSubmit = template.NameForSubmit()
				} else {
					templateContent = string(tpl.LegacyBody())
				}
			}

			err = prShared.BodySurvey(&tb, templateContent, editorCommand)
			if err != nil {
				return
			}
		}

		openURL, err = generatePreviewURL(apiClient, baseRepo, tb)
		if err != nil {
			return
		}

		allowPreview := !tb.HasMetadata() && utils.ValidURL(openURL)
		action, err = prShared.ConfirmSubmission(allowPreview, repo.ViewerCanTriage())
		if err != nil {
			err = fmt.Errorf("unable to confirm: %w", err)
			return
		}

		if action == prShared.MetadataAction {
			fetcher := &prShared.MetadataFetcher{
				IO:        opts.IO,
				APIClient: apiClient,
				Repo:      baseRepo,
				State:     &tb,
			}
			err = prShared.MetadataSurvey(opts.IO, baseRepo, fetcher, &tb)
			if err != nil {
				return
			}

			action, err = prShared.ConfirmSubmission(!tb.HasMetadata(), false)
			if err != nil {
				return
			}
		}

		if action == prShared.CancelAction {
			fmt.Fprintln(opts.IO.ErrOut, "Discarding.")
			err = cmdutil.CancelError
			return
		}
	} else {
		if tb.Title == "" {
			err = fmt.Errorf("title can't be blank")
			return
		}
	}

	if action == prShared.PreviewAction {
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	} else if action == prShared.SubmitAction {
		params := map[string]interface{}{
			"title": tb.Title,
			"body":  tb.Body,
		}
		if templateNameForSubmit != "" {
			params["issueTemplate"] = templateNameForSubmit
		}

		err = prShared.AddMetadataToIssueParams(apiClient, baseRepo, params, &tb)
		if err != nil {
			return
		}

		var newIssue *api.Issue
		newIssue, err = api.IssueCreate(apiClient, repo, params)
		if err != nil {
			return
		}

		fmt.Fprintln(opts.IO.Out, newIssue.URL)
	} else {
		panic("Unreachable state")
	}

	return
}

func generatePreviewURL(apiClient *api.Client, baseRepo ghrepo.Interface, tb shared.IssueMetadataState) (string, error) {
	openURL := ghrepo.GenerateRepoURL(baseRepo, "issues/new")
	return prShared.WithPrAndIssueQueryParams(apiClient, baseRepo, openURL, tb)
}
