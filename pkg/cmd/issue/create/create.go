package create

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/text"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient       func() (*http.Client, error)
	Config           func() (gh.Config, error)
	IO               *iostreams.IOStreams
	BaseRepo         func() (ghrepo.Interface, error)
	Browser          browser.Browser
	Prompter         prShared.Prompt
	Detector         fd.Detector
	TitledEditSurvey func(string, string) (string, string, error)

	RootDirOverride string

	HasRepoOverride bool
	EditorMode      bool
	WebMode         bool
	RecoverFile     string

	Title       string
	Body        string
	Interactive bool

	Assignees []string
	Labels    []string
	Projects  []string
	Milestone string
	Template  string

	IssueType   string
	issueTypeID string // resolved during interactive flow to avoid double API call
	Parent      string
	BlockedBy   []string
	Blocking    []string
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		Prompter:   f.Prompter,

		TitledEditSurvey: prShared.TitledEditSurvey(&prShared.UserEditor{Config: f.Config, IO: f.IOStreams}),
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
		Long: heredoc.Docf(`
			Create an issue on GitHub.

			Adding an issue to projects requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.

			The %[1]s--assignee%[1]s flag supports the following special values:
			- %[1]s@me%[1]s: assign yourself
			- %[1]s@copilot%[1]s: assign Copilot (not supported on GitHub Enterprise Server)
		`, "`"),
		Example: heredoc.Doc(`
			$ gh issue create --title "I found a bug" --body "Nothing works"
			$ gh issue create --label "bug,help wanted"
			$ gh issue create --label bug --label "help wanted"
			$ gh issue create --assignee monalisa,hubot
			$ gh issue create --assignee "@me"
			$ gh issue create --assignee "@copilot"
			$ gh issue create --project "Roadmap"
			$ gh issue create --template "Bug Report"
			$ gh issue create --type Bug
			$ gh issue create --parent 100
			$ gh issue create --parent https://github.com/cli/go-gh/issues/42
			$ gh issue create --blocked-by 200,201 --blocking 300
		`),
		Args:    cmdutil.NoArgsQuoteReminder,
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			var err error
			opts.EditorMode, err = prShared.InitEditorMode(f, opts.EditorMode, opts.WebMode, opts.IO.CanPrompt())
			if err != nil {
				return err
			}

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
				return cmdutil.FlagErrorf("`--recover` only supported when running interactively")
			}

			if opts.Template != "" && bodyProvided {
				return errors.New("`--template` is not supported when using `--body` or `--body-file`")
			}

			opts.Interactive = !opts.EditorMode && !(titleProvided && bodyProvided)

			if opts.Interactive && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("must provide `--title` and `--body` when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Supply a title. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolVarP(&opts.EditorMode, "editor", "e", false, "Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the browser to create an issue")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their `login`. Use \"@me\" to self-assign.")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVarP(&opts.Projects, "project", "p", nil, "Add the issue to projects by `title`")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the issue to a milestone by `name`")
	cmd.Flags().StringVar(&opts.RecoverFile, "recover", "", "Recover input from a failed run of create")
	cmd.Flags().StringVarP(&opts.Template, "template", "T", "", "Template `name` to use as starting body text")
	cmd.Flags().StringVar(&opts.IssueType, "type", "", "Set the issue type by `name`")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "Add the new issue as a sub-issue of the specified parent `number` or URL")
	cmd.Flags().StringSliceVar(&opts.BlockedBy, "blocked-by", nil, "Mark the new issue as blocked by these issue `numbers` or URLs")
	cmd.Flags().StringSliceVar(&opts.Blocking, "blocking", nil, "Mark the new issue as blocking these issue `numbers` or URLs")

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

	// TODO projectsV1Deprecation
	// Remove this section as we should no longer need to detect
	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, baseRepo.RepoHost())
	}

	projectsV1Support := opts.Detector.ProjectsV1()
	issueFeatures, err := opts.Detector.IssueFeatures()
	if err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()

	var milestones []string
	if opts.Milestone != "" {
		milestones = []string{opts.Milestone}
	}

	// Replace special values in assignees
	// For web mode, @copilot should be replaced by name; otherwise, login.
	assigneeReplacer := prShared.NewSpecialAssigneeReplacer(apiClient, baseRepo.RepoHost(), issueFeatures.ApiActorsSupported, !opts.WebMode)
	assignees, err := assigneeReplacer.ReplaceSlice(opts.Assignees)
	if err != nil {
		return err
	}
	assigneeSet := set.NewStringSet()
	assigneeSet.AddValues(assignees)

	tb := prShared.IssueMetadataState{
		Type:               prShared.IssueMetadata,
		ApiActorsSupported: issueFeatures.ApiActorsSupported, // TODO ApiActorsSupported
		Assignees:          assigneeSet.ToSlice(),
		Labels:             opts.Labels,
		ProjectTitles:      opts.Projects,
		Milestones:         milestones,
		Title:              opts.Title,
		Body:               opts.Body,
	}

	if opts.RecoverFile != "" {
		err = prShared.FillFromJSON(opts.IO, opts.RecoverFile, &tb)
		if err != nil {
			err = fmt.Errorf("failed to recover input: %w", err)
			return
		}
	}

	tpl := prShared.NewTemplateManager(httpClient, baseRepo, opts.Prompter, opts.RootDirOverride, !opts.HasRepoOverride, false)

	if opts.WebMode {
		var openURL string
		if opts.Title != "" || opts.Body != "" || tb.HasMetadata() {
			openURL, err = generatePreviewURL(apiClient, baseRepo, tb, projectsV1Support)
			if err != nil {
				return
			}
			if !prShared.ValidURL(openURL) {
				err = fmt.Errorf("cannot open in browser: maximum URL length exceeded")
				return
			}
		} else if ok, _ := tpl.HasTemplates(); ok {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new/choose")
		} else {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new")
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "\nCreating issue in %s\n\n", ghrepo.FullName(baseRepo))
	}

	repo, err := api.IssueRepoInfo(apiClient, baseRepo)
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
		defer prShared.PreserveInput(opts.IO, &tb, &err)()

		if opts.Title == "" {
			err = prShared.TitleSurvey(opts.Prompter, opts.IO, &tb)
			if err != nil {
				return
			}
		}

		if opts.Body == "" {
			templateContent := ""

			if opts.RecoverFile == "" {
				var template prShared.Template

				if opts.Template != "" {
					template, err = tpl.Select(opts.Template)
					if err != nil {
						return
					}
				} else {
					template, err = tpl.Choose()
					if err != nil {
						return
					}
				}

				if template != nil {
					templateContent = string(template.Body())
					templateNameForSubmit = template.NameForSubmit()
				} else {
					templateContent = string(tpl.LegacyBody())
				}
			}

			err = prShared.BodySurvey(opts.Prompter, &tb, templateContent)
			if err != nil {
				return
			}
		}

		// Interactive issue type selection
		if opts.IssueType == "" {
			issueTypes, typesErr := api.RepoIssueTypes(apiClient, baseRepo)
			if typesErr == nil && len(issueTypes) > 0 {
				typeNames := make([]string, len(issueTypes))
				for i, t := range issueTypes {
					typeNames[i] = t.Name
				}
				var selected int
				selected, err = opts.Prompter.Select("Issue type", "", typeNames)
				if err != nil {
					return
				}
				opts.IssueType = typeNames[selected]
				opts.issueTypeID = issueTypes[selected].ID
			}
		}

		openURL, err = generatePreviewURL(apiClient, baseRepo, tb, projectsV1Support)
		if err != nil {
			return
		}

		allowPreview := !tb.HasMetadata() && prShared.ValidURL(openURL)
		action, err = prShared.ConfirmIssueSubmission(opts.Prompter, allowPreview, repo.ViewerCanTriage())
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
			var assigneeSearchFunc func(string) prompter.MultiSelectSearchResult
			if issueFeatures.ApiActorsSupported {
				assigneeSearchFunc = prShared.RepoAssigneeSearchFunc(apiClient, baseRepo)
			}
			err = prShared.MetadataSurvey(opts.Prompter, opts.IO, baseRepo, fetcher, &tb, projectsV1Support, nil, assigneeSearchFunc)
			if err != nil {
				return
			}

			action, err = prShared.ConfirmIssueSubmission(opts.Prompter, !tb.HasMetadata(), false)
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
		if opts.EditorMode {
			if opts.Template != "" {
				var template prShared.Template
				template, err = tpl.Select(opts.Template)
				if err != nil {
					return
				}
				if tb.Title == "" {
					tb.Title = template.Title()
				}
				templateNameForSubmit = template.NameForSubmit()
				tb.Body = string(template.Body())
			}

			tb.Title, tb.Body, err = opts.TitledEditSurvey(tb.Title, tb.Body)
			if err != nil {
				return
			}
		}
		if tb.Title == "" {
			err = fmt.Errorf("title can't be blank")
			return
		}
	}

	if action == prShared.PreviewAction {
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
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

		err = prShared.AddMetadataToIssueParams(apiClient, baseRepo, params, &tb, projectsV1Support)
		if err != nil {
			return
		}

		var newIssue *api.Issue
		newIssue, err = api.IssueCreate(apiClient, repo, params)
		if err != nil {
			return
		}

		var updateOpts api.DeferredUpdateIssueOptions
		updateOpts, err = deferredUpdateIssueOptions(apiClient, baseRepo, newIssue, opts)
		if err != nil {
			return
		}
		if err = api.DeferredUpdateIssue(apiClient, updateOpts); err != nil {
			return
		}

		fmt.Fprintln(opts.IO.Out, newIssue.URL)
	} else {
		panic("Unreachable state")
	}

	return
}

func generatePreviewURL(apiClient *api.Client, baseRepo ghrepo.Interface, tb prShared.IssueMetadataState, projectsV1Support gh.ProjectsV1Support) (string, error) {
	openURL := ghrepo.GenerateRepoURL(baseRepo, "issues/new")
	return prShared.WithPrAndIssueQueryParams(apiClient, baseRepo, openURL, tb, projectsV1Support)
}

// deferredUpdateIssueOptions resolves the user-supplied --type / --parent /
// --blocked-by / --blocking flags into the IDs that DeferredUpdateIssue
// expects.
func deferredUpdateIssueOptions(client *api.Client, baseRepo ghrepo.Interface, issue *api.Issue, opts *CreateOptions) (api.DeferredUpdateIssueOptions, error) {
	updateOpts := api.DeferredUpdateIssueOptions{
		IssueID:  issue.ID,
		Hostname: baseRepo.RepoHost(),
	}

	if opts.IssueType != "" {
		typeID := opts.issueTypeID
		if typeID == "" {
			var err error
			typeID, err = issueShared.ResolveIssueTypeName(client, baseRepo, opts.IssueType)
			if err != nil {
				return updateOpts, err
			}
		}
		updateOpts.IssueTypeID = typeID
	}

	if opts.Parent != "" {
		parentID, err := issueShared.ResolveIssueRef(client, baseRepo, opts.Parent)
		if err != nil {
			return updateOpts, fmt.Errorf("resolving --parent reference %q: %w", opts.Parent, err)
		}
		updateOpts.ParentID = parentID
	}

	for _, ref := range opts.BlockedBy {
		id, err := issueShared.ResolveIssueRef(client, baseRepo, ref)
		if err != nil {
			return updateOpts, fmt.Errorf("resolving --blocked-by reference %q: %w", ref, err)
		}
		updateOpts.AddBlockedByIDs = append(updateOpts.AddBlockedByIDs, id)
	}

	for _, ref := range opts.Blocking {
		id, err := issueShared.ResolveIssueRef(client, baseRepo, ref)
		if err != nil {
			return updateOpts, fmt.Errorf("resolving --blocking reference %q: %w", ref, err)
		}
		updateOpts.AddBlockingIDs = append(updateOpts.AddBlockingIDs, id)
	}

	return updateOpts, nil
}
