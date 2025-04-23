package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	// This struct stores user input and factory functions
	HttpClient       func() (*http.Client, error)
	GitClient        *git.Client
	Config           func() (gh.Config, error)
	IO               *iostreams.IOStreams
	Remotes          func() (ghContext.Remotes, error)
	Branch           func() (string, error)
	Browser          browser.Browser
	Prompter         shared.Prompt
	Finder           shared.PRFinder
	TitledEditSurvey func(string, string) (string, string, error)

	TitleProvided bool
	BodyProvided  bool

	RootDirOverride string
	RepoOverride    string

	Autofill    bool
	FillVerbose bool
	FillFirst   bool
	EditorMode  bool
	WebMode     bool
	RecoverFile string

	IsDraft    bool
	Title      string
	Body       string
	BaseBranch string
	HeadBranch string

	Reviewers []string
	Assignees []string
	Labels    []string
	Projects  []string
	Milestone string

	MaintainerCanModify bool
	Template            string

	DryRun bool
}

// creationRefs is an interface that provides the necessary information for creating a pull request in the API.
// Upcasting to concrete implementations can provide further context on other operations (forking and pushing).
type creationRefs interface {
	// QualifiedHeadRef returns a stringified form of the head ref, varying depending
	// on whether the head ref is in the same repository as the base ref. If they are
	// the same repository, we return the branch name only. If they are different repositories,
	// we return the owner and branch name in the form <owner>:<branch>.
	QualifiedHeadRef() string
	// UnqualifiedHeadRef returns a head ref in the form of the branch name only.
	UnqualifiedHeadRef() string
	//BaseRef returns the base branch name.
	BaseRef() string

	// While the only thing really required from an api.Repository is the repository ID, changing that
	// would require changing the API function signatures, and the refactor that introduced this refs
	// type is already large enough.
	BaseRepo() *api.Repository
}

type baseRefs struct {
	baseRepo       *api.Repository
	baseBranchName string
}

func (r baseRefs) BaseRef() string {
	return r.baseBranchName
}

func (r baseRefs) BaseRepo() *api.Repository {
	return r.baseRepo
}

// skipPushRefs indicate to handlePush that no pushing is required.
type skipPushRefs struct {
	baseRefs

	qualifiedHeadRef shared.QualifiedHeadRef
}

func (r skipPushRefs) QualifiedHeadRef() string {
	return r.qualifiedHeadRef.String()
}

func (r skipPushRefs) UnqualifiedHeadRef() string {
	return r.qualifiedHeadRef.BranchName()
}

// pushableRefs indicate to handlePush that pushing is required,
// and provide further information (HeadRepo) on where that push
// should go.
type pushableRefs struct {
	baseRefs

	headRepo       ghrepo.Interface
	headBranchName string
}

func (r pushableRefs) QualifiedHeadRef() string {
	if ghrepo.IsSame(r.headRepo, r.baseRepo) {
		return r.headBranchName
	}
	return fmt.Sprintf("%s:%s", r.headRepo.RepoOwner(), r.headBranchName)
}

func (r pushableRefs) UnqualifiedHeadRef() string {
	return r.headBranchName
}

func (r pushableRefs) HeadRepo() ghrepo.Interface {
	return r.headRepo
}

// forkableRefs indicate to handlePush that forking is required before
// pushing. The expectation is that after forking, this is converted to
// pushableRefs. We could go very OOP and have a Fork method on this
// struct that returns a pushableRefs but then we'd need to embed an API client
// and it just seems nice that it is a simple bag of data.
type forkableRefs struct {
	baseRefs

	qualifiedHeadRef shared.QualifiedHeadRef
}

func (r forkableRefs) QualifiedHeadRef() string {
	return r.qualifiedHeadRef.String()
}

func (r forkableRefs) UnqualifiedHeadRef() string {
	return r.qualifiedHeadRef.BranchName()
}

// CreateContext stores contextual data about the creation process and is for building up enough
// data to create a pull request.
type CreateContext struct {
	ResolvedRemotes *ghContext.ResolvedRemotes
	PRRefs          creationRefs
	// BaseTrackingBranch is perhaps a slightly leaky abstraction in the presence
	// of PRRefs, but a huge amount of refactoring was done to introduce that struct,
	// and this is a small price to pay for the convenience of not having to do a lot
	// more design.
	BaseTrackingBranch string
	Client             *api.Client
	GitClient          *git.Client
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:               f.IOStreams,
		HttpClient:       f.HttpClient,
		GitClient:        f.GitClient,
		Config:           f.Config,
		Remotes:          f.Remotes,
		Branch:           f.Branch,
		Browser:          f.Browser,
		Prompter:         f.Prompter,
		TitledEditSurvey: shared.TitledEditSurvey(&shared.UserEditor{Config: f.Config, IO: f.IOStreams}),
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: heredoc.Docf(`
			Create a pull request on GitHub.

			When the current branch isn't fully pushed to a git remote, a prompt will ask where
			to push the branch and offer an option to fork the base repository. Use %[1]s--head%[1]s to
			explicitly skip any forking or pushing behavior.

			%[1]s--head%[1]s supports %[1]s<user>:<branch>%[1]s syntax to select a head repo owned by %[1]s<user>%[1]s.
			Using an organization as the %[1]s<user>%[1]s is currently not supported.
			For more information, see <https://github.com/cli/cli/issues/10093>

			A prompt will also ask for the title and the body of the pull request. Use %[1]s--title%[1]s and
			%[1]s--body%[1]s to skip this, or use %[1]s--fill%[1]s to autofill these values from git commits.
			It's important to notice that if the %[1]s--title%[1]s and/or %[1]s--body%[1]s are also provided
			alongside %[1]s--fill%[1]s, the values specified by %[1]s--title%[1]s and/or %[1]s--body%[1]s will
			take precedence and overwrite any autofilled content.

			The base branch for the created PR can be specified using the %[1]s--base%[1]s flag. If not provided,
			the value of %[1]sgh-merge-base%[1]s git branch config will be used. If not configured, the repository's
			default branch will be used. Run %[1]sgit config branch.{current}.gh-merge-base {base}%[1]s to configure
			the current branch to use the specified merge base.

			Link an issue to the pull request by referencing the issue in the body of the pull
			request. If the body text mentions %[1]sFixes #123%[1]s or %[1]sCloses #123%[1]s, the referenced issue
			will automatically get closed when the pull request gets merged.

			By default, users with write access to the base repository can push new commits to the
			head branch of the pull request. Disable this with %[1]s--no-maintainer-edit%[1]s.

			Adding a pull request to projects requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh pr create --title "The bug is fixed" --body "Everything works again"
			$ gh pr create --reviewer monalisa,hubot  --reviewer myorg/team-name
			$ gh pr create --project "Roadmap"
			$ gh pr create --base develop --head monalisa:feature
			$ gh pr create --template "pull_request_template.md"
		`),
		Args:    cmdutil.NoArgsQuoteReminder,
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			opts.TitleProvided = cmd.Flags().Changed("title")
			opts.RepoOverride, _ = cmd.Flags().GetString("repo")
			// Workaround: Due to the way this command is implemented, we need to manually check GH_REPO.
			// Commands should use the standard BaseRepoOverride functionality to handle this behavior instead.
			if opts.RepoOverride == "" {
				opts.RepoOverride = os.Getenv("GH_REPO")
			}

			noMaintainerEdit, _ := cmd.Flags().GetBool("no-maintainer-edit")
			opts.MaintainerCanModify = !noMaintainerEdit

			if !opts.IO.CanPrompt() && opts.RecoverFile != "" {
				return cmdutil.FlagErrorf("`--recover` only supported when running interactively")
			}

			if opts.IsDraft && opts.WebMode {
				return cmdutil.FlagErrorf("the `--draft` flag is not supported with `--web`")
			}

			if len(opts.Reviewers) > 0 && opts.WebMode {
				return cmdutil.FlagErrorf("the `--reviewer` flag is not supported with `--web`")
			}

			if cmd.Flags().Changed("no-maintainer-edit") && opts.WebMode {
				return cmdutil.FlagErrorf("the `--no-maintainer-edit` flag is not supported with `--web`")
			}

			if opts.Autofill && opts.FillFirst {
				return cmdutil.FlagErrorf("`--fill` is not supported with `--fill-first`")
			}

			if opts.FillVerbose && opts.FillFirst {
				return cmdutil.FlagErrorf("`--fill-verbose` is not supported with `--fill-first`")
			}

			if opts.FillVerbose && opts.Autofill {
				return cmdutil.FlagErrorf("`--fill-verbose` is not supported with `--fill`")
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--editor` or `--web`",
				opts.EditorMode,
				opts.WebMode,
			); err != nil {
				return err
			}

			var err error
			opts.EditorMode, err = shared.InitEditorMode(f, opts.EditorMode, opts.WebMode, opts.IO.CanPrompt())
			if err != nil {
				return err
			}

			opts.BodyProvided = cmd.Flags().Changed("body")
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
				opts.BodyProvided = true
			}

			if opts.Template != "" && opts.BodyProvided {
				return cmdutil.FlagErrorf("`--template` is not supported when using `--body` or `--body-file`")
			}

			if !opts.IO.CanPrompt() && !opts.WebMode && !(opts.FillVerbose || opts.Autofill || opts.FillFirst) && (!opts.TitleProvided || !opts.BodyProvided) {
				return cmdutil.FlagErrorf("must provide `--title` and `--body` (or `--fill` or `fill-first` or `--fillverbose`) when not running interactively")
			}

			if opts.DryRun && opts.WebMode {
				return cmdutil.FlagErrorf("`--dry-run` is not supported when using `--web`")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	fl := cmd.Flags()
	fl.BoolVarP(&opts.IsDraft, "draft", "d", false, "Mark pull request as a draft")
	fl.StringVarP(&opts.Title, "title", "t", "", "Title for the pull request")
	fl.StringVarP(&opts.Body, "body", "b", "", "Body for the pull request")
	fl.StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	fl.StringVarP(&opts.BaseBranch, "base", "B", "", "The `branch` into which you want your code merged")
	fl.StringVarP(&opts.HeadBranch, "head", "H", "", "The `branch` that contains commits for your pull request (default [current branch])")
	fl.BoolVarP(&opts.EditorMode, "editor", "e", false, "Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.")
	fl.BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to create a pull request")
	fl.BoolVarP(&opts.FillVerbose, "fill-verbose", "", false, "Use commits msg+body for description")
	fl.BoolVarP(&opts.Autofill, "fill", "f", false, "Use commit info for title and body")
	fl.BoolVar(&opts.FillFirst, "fill-first", false, "Use first commit info for title and body")
	fl.StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people or teams by their `handle`")
	fl.StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their `login`. Use \"@me\" to self-assign.")
	fl.StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by `name`")
	fl.StringSliceVarP(&opts.Projects, "project", "p", nil, "Add the pull request to projects by `title`")
	fl.StringVarP(&opts.Milestone, "milestone", "m", "", "Add the pull request to a milestone by `name`")
	fl.Bool("no-maintainer-edit", false, "Disable maintainer's ability to modify pull request")
	fl.StringVar(&opts.RecoverFile, "recover", "", "Recover input from a failed run of create")
	fl.StringVarP(&opts.Template, "template", "T", "", "Template `file` to use as starting body text")
	fl.BoolVar(&opts.DryRun, "dry-run", false, "Print details instead of creating the PR. May still push git changes.")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "base", "head")

	_ = cmd.RegisterFlagCompletionFunc("reviewer", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		results, err := requestableReviewersForCompletion(opts)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func createRun(opts *CreateOptions) error {
	ctx, err := NewCreateContext(opts)
	if err != nil {
		return err
	}

	client := ctx.Client

	state, err := NewIssueState(*ctx, *opts)
	if err != nil {
		return err
	}

	var openURL string

	if opts.WebMode {
		if !(opts.Autofill || opts.FillFirst) {
			state.Title = opts.Title
			state.Body = opts.Body
		}
		if opts.Template != "" {
			state.Template = opts.Template
		}
		err = handlePush(*opts, *ctx)
		if err != nil {
			return err
		}
		openURL, err = generateCompareURL(*ctx, *state)
		if err != nil {
			return err
		}
		if !shared.ValidURL(openURL) {
			err = fmt.Errorf("cannot open in browser: maximum URL length exceeded")
			return err
		}
		return previewPR(*opts, openURL)
	}

	if opts.TitleProvided {
		state.Title = opts.Title
	}

	if opts.BodyProvided {
		state.Body = opts.Body
	}

	existingPR, _, err := opts.Finder.Find(shared.FindOptions{
		Selector:   ctx.PRRefs.QualifiedHeadRef(),
		BaseBranch: ctx.PRRefs.BaseRef(),
		States:     []string{"OPEN"},
		Fields:     []string{"url"},
	})
	var notFound *shared.NotFoundError
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("error checking for existing pull request: %w", err)
	}
	if err == nil {
		return fmt.Errorf("a pull request for branch %q into branch %q already exists:\n%s",
			ctx.PRRefs.QualifiedHeadRef(), ctx.PRRefs.BaseRef(), existingPR.URL)
	}

	message := "\nCreating pull request for %s into %s in %s\n\n"
	if state.Draft {
		message = "\nCreating draft pull request for %s into %s in %s\n\n"
	}
	if opts.DryRun {
		message = "\nDry Running pull request for %s into %s in %s\n\n"
	}

	cs := opts.IO.ColorScheme()

	if opts.IO.CanPrompt() {
		fmt.Fprintf(opts.IO.ErrOut, message,
			cs.Cyan(ctx.PRRefs.QualifiedHeadRef()),
			cs.Cyan(ctx.PRRefs.BaseRef()),
			ghrepo.FullName(ctx.PRRefs.BaseRepo()))
	}

	if !opts.EditorMode && (opts.FillVerbose || opts.Autofill || opts.FillFirst || (opts.TitleProvided && opts.BodyProvided)) {
		err = handlePush(*opts, *ctx)
		if err != nil {
			return err
		}
		// TODO wm: revisit project support
		return submitPR(*opts, *ctx, *state, gh.ProjectsV1Supported)
	}

	if opts.RecoverFile != "" {
		err = shared.FillFromJSON(opts.IO, opts.RecoverFile, state)
		if err != nil {
			return fmt.Errorf("failed to recover input: %w", err)
		}
	}

	action := shared.SubmitAction
	if opts.IsDraft {
		action = shared.SubmitDraftAction
	}

	tpl := shared.NewTemplateManager(client.HTTP(), ctx.PRRefs.BaseRepo(), opts.Prompter, opts.RootDirOverride, opts.RepoOverride == "", true)

	if opts.EditorMode {
		if opts.Template != "" {
			var template shared.Template
			template, err = tpl.Select(opts.Template)
			if err != nil {
				return err
			}
			if state.Title == "" {
				state.Title = template.Title()
			}
			state.Body = string(template.Body())
		}

		state.Title, state.Body, err = opts.TitledEditSurvey(state.Title, state.Body)
		if err != nil {
			return err
		}
		if state.Title == "" {
			err = fmt.Errorf("title can't be blank")
			return err
		}
	} else {

		if !opts.TitleProvided {
			err = shared.TitleSurvey(opts.Prompter, opts.IO, state)
			if err != nil {
				return err
			}
		}

		defer shared.PreserveInput(opts.IO, state, &err)()

		if !opts.BodyProvided {
			templateContent := ""
			if opts.RecoverFile == "" {
				var template shared.Template

				if opts.Template != "" {
					template, err = tpl.Select(opts.Template)
					if err != nil {
						return err
					}
				} else {
					template, err = tpl.Choose()
					if err != nil {
						return err
					}
				}

				if template != nil {
					templateContent = string(template.Body())
				}
			}

			err = shared.BodySurvey(opts.Prompter, state, templateContent)
			if err != nil {
				return err
			}
		}

		openURL, err = generateCompareURL(*ctx, *state)
		if err != nil {
			return err
		}

		allowPreview := !state.HasMetadata() && shared.ValidURL(openURL) && !opts.DryRun
		allowMetadata := ctx.PRRefs.BaseRepo().ViewerCanTriage()
		action, err = shared.ConfirmPRSubmission(opts.Prompter, allowPreview, allowMetadata, state.Draft)
		if err != nil {
			return fmt.Errorf("unable to confirm: %w", err)
		}

		if action == shared.MetadataAction {
			fetcher := &shared.MetadataFetcher{
				IO:        opts.IO,
				APIClient: client,
				Repo:      ctx.PRRefs.BaseRepo(),
				State:     state,
			}
			// TODO wm: revisit project support
			err = shared.MetadataSurvey(opts.Prompter, opts.IO, ctx.PRRefs.BaseRepo(), fetcher, state, gh.ProjectsV1Supported)
			if err != nil {
				return err
			}

			action, err = shared.ConfirmPRSubmission(opts.Prompter, !state.HasMetadata() && !opts.DryRun, false, state.Draft)
			if err != nil {
				return err
			}
		}
	}

	if action == shared.CancelAction {
		fmt.Fprintln(opts.IO.ErrOut, "Discarding.")
		err = cmdutil.CancelError
		return err
	}

	err = handlePush(*opts, *ctx)
	if err != nil {
		return err
	}

	if action == shared.PreviewAction {
		return previewPR(*opts, openURL)
	}

	if action == shared.SubmitDraftAction {
		state.Draft = true
		// TODO wm: revisit project support
		return submitPR(*opts, *ctx, *state, gh.ProjectsV1Supported)
	}

	if action == shared.SubmitAction {
		// TODO wm: revisit project support
		return submitPR(*opts, *ctx, *state, gh.ProjectsV1Supported)
	}

	err = errors.New("expected to cancel, preview, or submit")
	return err
}

var regexPattern = regexp.MustCompile(`(?m)^`)

func initDefaultTitleBody(ctx CreateContext, state *shared.IssueMetadataState, useFirstCommit bool, addBody bool) error {
	commits, err := ctx.GitClient.Commits(context.Background(), ctx.BaseTrackingBranch, ctx.PRRefs.UnqualifiedHeadRef())
	if err != nil {
		return err
	}

	if len(commits) == 1 || useFirstCommit {
		state.Title = commits[len(commits)-1].Title
		state.Body = commits[len(commits)-1].Body
	} else {
		state.Title = humanize(ctx.PRRefs.UnqualifiedHeadRef())
		var body strings.Builder
		for i := len(commits) - 1; i >= 0; i-- {
			fmt.Fprintf(&body, "- **%s**\n", commits[i].Title)
			if addBody {
				x := regexPattern.ReplaceAllString(commits[i].Body, "  ")
				fmt.Fprintf(&body, "%s", x)

				if i > 0 {
					fmt.Fprintln(&body)
					fmt.Fprintln(&body)
				}
			}
		}
		state.Body = body.String()
	}

	return nil
}

func NewIssueState(ctx CreateContext, opts CreateOptions) (*shared.IssueMetadataState, error) {
	var milestoneTitles []string
	if opts.Milestone != "" {
		milestoneTitles = []string{opts.Milestone}
	}

	meReplacer := shared.NewMeReplacer(ctx.Client, ctx.PRRefs.BaseRepo().RepoHost())
	assignees, err := meReplacer.ReplaceSlice(opts.Assignees)
	if err != nil {
		return nil, err
	}

	state := &shared.IssueMetadataState{
		Type:          shared.PRMetadata,
		Reviewers:     opts.Reviewers,
		Assignees:     assignees,
		Labels:        opts.Labels,
		ProjectTitles: opts.Projects,
		Milestones:    milestoneTitles,
		Draft:         opts.IsDraft,
	}

	if opts.FillVerbose || opts.Autofill || opts.FillFirst || !opts.TitleProvided || !opts.BodyProvided {
		err := initDefaultTitleBody(ctx, state, opts.FillFirst, opts.FillVerbose)
		if err != nil && (opts.FillVerbose || opts.Autofill || opts.FillFirst) {
			return nil, fmt.Errorf("could not compute title or body defaults: %w", err)
		}
	}

	return state, nil
}

func NewCreateContext(opts *CreateOptions) (*CreateContext, error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil, err
	}
	client := api.NewClientFromHTTP(httpClient)

	remotes, err := getRemotes(opts)
	if err != nil {
		return nil, err
	}

	resolvedRemotes, err := ghContext.ResolveRemotesToRepos(remotes, client, opts.RepoOverride)
	if err != nil {
		return nil, err
	}

	var baseRepo *api.Repository
	if br, err := resolvedRemotes.BaseRepo(opts.IO); err == nil {
		if r, ok := br.(*api.Repository); ok {
			baseRepo = r
		} else {
			// TODO: if RepoNetwork is going to be requested anyway in `repoContext.HeadRepos()`,
			// consider piggybacking on that result instead of performing a separate lookup
			baseRepo, err = api.GitHubRepo(client, br)
			if err != nil {
				return nil, err
			}
		}
	} else {
		return nil, err
	}

	// This closure provides an easy way to instantiate a CreateContext with everything other than
	// the refs. This probably indicates that CreateContext could do with some rework, but the refactor
	// to introduce PRRefs is already large enough.
	var newCreateContext = func(refs creationRefs) *CreateContext {
		baseTrackingBranch := refs.BaseRef()

		// The baseTrackingBranch is used later for a command like:
		// `git commit upstream/main feature` in order to create a PR message showing the commits
		// between these two refs. I'm not really sure what is expected to happen if we don't have a remote,
		// which seems like it would be possible with a command `gh pr create --repo owner/repo-that-is-not-a-remote`.
		// In that case, we might just have a mess? In any case, this is what the old code did, so I don't want to change
		// it as part of an already large refactor.
		baseRemote, _ := resolvedRemotes.RemoteForRepo(baseRepo)
		if baseRemote != nil {
			baseTrackingBranch = fmt.Sprintf("%s/%s", baseRemote.Name, baseTrackingBranch)
		}

		return &CreateContext{
			ResolvedRemotes:    resolvedRemotes,
			Client:             client,
			GitClient:          opts.GitClient,
			PRRefs:             refs,
			BaseTrackingBranch: baseTrackingBranch,
		}
	}

	// If the user provided a head branch we're going to use that without any interrogation
	// of git. The value can take the form of <branch> or <user>:<branch>. In the former case, the
	// PR base and head repos are the same. In the latter case we don't know the head repo
	// (though we could look it up in the API) but fortunately we don't need to because the API
	// will resolve this for us when we create the pull request. This is possible because
	// users can only have a single fork in their namespace, and organizations don't work at all with this ref format.
	//
	// Note that providing the head branch in this way indicates that we shouldn't push the branch,
	// and we indicate that via the returned type as well.
	if opts.HeadBranch != "" {
		qualifiedHeadRef, err := shared.ParseQualifiedHeadRef(opts.HeadBranch)
		if err != nil {
			return nil, err
		}

		branchConfig, err := opts.GitClient.ReadBranchConfig(context.Background(), qualifiedHeadRef.BranchName())
		if err != nil {
			return nil, err
		}

		baseBranch := opts.BaseBranch
		if baseBranch == "" {
			baseBranch = branchConfig.MergeBase
		}
		if baseBranch == "" {
			baseBranch = baseRepo.DefaultBranchRef.Name
		}

		return newCreateContext(skipPushRefs{
			qualifiedHeadRef: qualifiedHeadRef,
			baseRefs: baseRefs{
				baseRepo:       baseRepo,
				baseBranchName: baseBranch,
			},
		}), nil
	}

	if ucc, err := opts.GitClient.UncommittedChangeCount(context.Background()); err == nil && ucc > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "Warning: %s\n", text.Pluralize(ucc, "uncommitted change"))
	}

	// If the user didn't provide a head branch then we're gettin' real. We're going to interrogate git
	// and try to create refs that are pushable.
	currentBranch, err := opts.Branch()
	if err != nil {
		return nil, fmt.Errorf("could not determine the current branch: %w", err)
	}

	branchConfig, err := opts.GitClient.ReadBranchConfig(context.Background(), currentBranch)
	if err != nil {
		return nil, err
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = branchConfig.MergeBase
	}
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}

	// First we check with the git information we have to see if we can figure out the default
	// head repo and remote branch name.
	defaultPRHead, err := shared.TryDetermineDefaultPRHead(
		// We requested the branch config already, so let's cache that
		shared.CachedBranchConfigGitConfigClient{
			CachedBranchConfig: branchConfig,
			GitConfigClient:    opts.GitClient,
		},
		shared.NewRemoteToRepoResolver(opts.Remotes),
		currentBranch,
	)
	if err != nil {
		return nil, err
	}

	// The baseRefs are always going to be the same from now on. If I could make this immutable I would!
	baseRefs := baseRefs{
		baseRepo:       baseRepo,
		baseBranchName: baseBranch,
	}

	// If we were able to determine a head repo, then let's check that the remote tracking ref matches the SHA of
	// HEAD. If it does, then we don't need to push, otherwise we'll need to ask the user to tell us where to push.
	if headRepo, present := defaultPRHead.Repo.Value(); present {
		// We may not find a remote because the git branch config may have a URL rather than a remote name.
		// Ideally, we would return a sentinel error from RemoteForRepo that we could compare to, but the
		// refactor that introduced this code was already large enough.
		headRemote, _ := resolvedRemotes.RemoteForRepo(headRepo)
		if headRemote != nil {
			resolvedRefs, _ := opts.GitClient.ShowRefs(
				context.Background(),
				[]string{
					"HEAD",
					fmt.Sprintf("refs/remotes/%s/%s", headRemote.Name, defaultPRHead.BranchName),
				},
			)

			// Two refs returned means we can compare HEAD to the remote tracking branch.
			// If we had a matching ref, then we can skip pushing.
			refsMatch := len(resolvedRefs) == 2 && resolvedRefs[0].Hash == resolvedRefs[1].Hash
			if refsMatch {
				qualifiedHeadRef := shared.NewQualifiedHeadRefWithoutOwner(defaultPRHead.BranchName)
				if headRepo.RepoOwner() != baseRepo.RepoOwner() {
					qualifiedHeadRef = shared.NewQualifiedHeadRef(headRepo.RepoOwner(), defaultPRHead.BranchName)
				}

				return newCreateContext(skipPushRefs{
					qualifiedHeadRef: qualifiedHeadRef,
					baseRefs:         baseRefs,
				}), nil
			}
		}
	}

	// If we didn't determine that the git indicated repo had the correct ref, we'll take a look at the other
	// remotes and see whether any of them have the same SHA as HEAD. Now, at this point, you might be asking yourself:
	// "Why didn't we collect all the SHAs with a single ShowRefs command above, for use in both cases?"
	// ...
	// That's because the code below has a bug that I've ported from the old code, in order to preserve the existing
	// behaviour, and to limit the scope of an already large refactor. The intention of the original code was to loop
	// over all the returned refs. However, as it turns out, our implementation of ShowRefs doesn't do that correctly.
	// Since it provides the --verify flag, git will return the SHAs for refs up until it hits a ref that doesn't exist,
	// at which point it bails out.
	//
	// Imagine you have a remotes "upstream" and "origin", and you have pushed your branch "feature" to "origin". Since
	// the order of remotes is always guaranteed "upstream", "github", "origin", and then everything else unstably sorted,
	// we will never get a SHA for origin, as refs/remotes/upstream/feature doesn't exist.
	//
	// Furthermore, when you really think about it, this code is a bit eager. What happens if you have the same SHA on
	// remotes "origin" and "colleague", this will always offer origin. If it were "colleague-a" and "colleague-b", no
	// order would be guaranteed between different invocations of pr create, because the order of remotes after "origin"
	// is unstable sorted.
	//
	// All that said, this has been the behaviour for a long, long time, and I do not want to make other behavioural changes
	// in what is mostly a refactor.
	refsToLookup := []string{"HEAD"}
	for _, remote := range remotes {
		refsToLookup = append(refsToLookup, fmt.Sprintf("refs/remotes/%s/%s", remote.Name, currentBranch))
	}

	// Ignoring the error in this case is allowed because we may get refs and an error (see: --verify flag above).
	// Ideally there would be a typed error to allow us to distinguish between an execution error and some refs
	// not existing. However, this is too much to take on in an already large refactor.
	refs, _ := opts.GitClient.ShowRefs(context.Background(), refsToLookup)
	if len(refs) > 1 {
		headRef := refs[0]
		var firstMatchingRef o.Option[git.RemoteTrackingRef]
		// Loop over all the refs, trying to find one that matches the SHA of HEAD.
		for _, r := range refs[1:] {
			if r.Hash == headRef.Hash {
				remoteTrackingRef, err := git.ParseRemoteTrackingRef(r.Name)
				if err != nil {
					return nil, err
				}

				firstMatchingRef = o.Some(remoteTrackingRef)
				break
			}
		}

		// If we found a matching ref, then we don't need to push.
		if ref, present := firstMatchingRef.Value(); present {
			remote, err := remotes.FindByName(ref.Remote)
			if err != nil {
				return nil, err
			}

			qualifiedHeadRef := shared.NewQualifiedHeadRefWithoutOwner(ref.Branch)
			if baseRepo.RepoOwner() != remote.RepoOwner() {
				qualifiedHeadRef = shared.NewQualifiedHeadRef(remote.RepoOwner(), ref.Branch)
			}

			return newCreateContext(skipPushRefs{
				qualifiedHeadRef: qualifiedHeadRef,
				baseRefs:         baseRefs,
			}), nil
		}
	}

	// If we haven't got a repo by now, and we can't prompt then it's game over.
	if !opts.IO.CanPrompt() {
		fmt.Fprintln(opts.IO.ErrOut, "aborted: you must first push the current branch to a remote, or use the --head flag")
		return nil, cmdutil.SilentError
	}

	// Otherwise, hooray, prompting!

	// First, we're going to look at our remotes and decide whether there are any repos we can push to.
	pushableRepos, err := resolvedRemotes.HeadRepos()
	if err != nil {
		return nil, err
	}

	// If we couldn't find any pushable repos, then find forks of the base repo.
	if len(pushableRepos) == 0 {
		pushableRepos, err = api.RepoFindForks(client, baseRepo, 3)
		if err != nil {
			return nil, err
		}
	}

	currentLogin, err := api.CurrentLoginName(client, baseRepo.RepoHost())
	if err != nil {
		return nil, err
	}

	hasOwnFork := false
	var pushOptions []string
	for _, r := range pushableRepos {
		pushOptions = append(pushOptions, ghrepo.FullName(r))
		if r.RepoOwner() == currentLogin {
			hasOwnFork = true
		}
	}

	if !hasOwnFork {
		pushOptions = append(pushOptions, fmt.Sprintf("Create a fork of %s", ghrepo.FullName(baseRepo)))
	}
	pushOptions = append(pushOptions, "Skip pushing the branch")
	pushOptions = append(pushOptions, "Cancel")

	selectedOption, err := opts.Prompter.Select(fmt.Sprintf("Where should we push the '%s' branch?", currentBranch), "", pushOptions)
	if err != nil {
		return nil, err
	}

	if selectedOption < len(pushableRepos) {
		// A repository has been selected to push to.
		return newCreateContext(pushableRefs{
			headRepo:       pushableRepos[selectedOption],
			headBranchName: currentBranch,
			baseRefs:       baseRefs,
		}), nil
	} else if pushOptions[selectedOption] == "Skip pushing the branch" {
		// We're going to skip pushing the branch altogether, meaning, use whatever SHA is already pushed.
		// It's not exactly clear what repo the user expects to use here for the HEAD, and maybe we should
		// make that clear in the UX somehow, but in the old implementation as far as I can tell, this
		// always meant "use the base repo".
		return newCreateContext(skipPushRefs{
			qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner(currentBranch),
			baseRefs:         baseRefs,
		}), nil
	} else if pushOptions[selectedOption] == "Cancel" {
		return nil, cmdutil.CancelError
	} else {
		// A fork should be created.
		return newCreateContext(forkableRefs{
			qualifiedHeadRef: shared.NewQualifiedHeadRef(currentLogin, currentBranch),
			baseRefs:         baseRefs,
		}), nil
	}
}

func getRemotes(opts *CreateOptions) (ghContext.Remotes, error) {
	// TODO: consider obtaining remotes from GitClient instead
	remotes, err := opts.Remotes()
	if err != nil {
		// When a repo override value is given, ignore errors when fetching git remotes
		// to support using this command outside of git repos.
		if opts.RepoOverride == "" {
			return nil, err
		}
	}
	return remotes, nil
}

func submitPR(opts CreateOptions, ctx CreateContext, state shared.IssueMetadataState, projectV1Support gh.ProjectsV1Support) error {
	client := ctx.Client

	params := map[string]interface{}{
		"title":               state.Title,
		"body":                state.Body,
		"draft":               state.Draft,
		"baseRefName":         ctx.PRRefs.BaseRef(),
		"headRefName":         ctx.PRRefs.QualifiedHeadRef(),
		"maintainerCanModify": opts.MaintainerCanModify,
	}

	if params["title"] == "" {
		return errors.New("pull request title must not be blank")
	}

	err := shared.AddMetadataToIssueParams(client, ctx.PRRefs.BaseRepo(), params, &state, projectV1Support)
	if err != nil {
		return err
	}

	if opts.DryRun {
		if opts.IO.IsStdoutTTY() {
			return renderPullRequestTTY(opts.IO, params, &state)
		} else {
			return renderPullRequestPlain(opts.IO.Out, params, &state)
		}
	}

	opts.IO.StartProgressIndicator()
	pr, err := api.CreatePullRequest(client, ctx.PRRefs.BaseRepo(), params)
	opts.IO.StopProgressIndicator()
	if pr != nil {
		fmt.Fprintln(opts.IO.Out, pr.URL)
	}
	if err != nil {
		if pr != nil {
			return fmt.Errorf("pull request update failed: %w", err)
		}
		return fmt.Errorf("pull request create failed: %w", err)
	}
	return nil
}

func renderPullRequestPlain(w io.Writer, params map[string]interface{}, state *shared.IssueMetadataState) error {
	fmt.Fprint(w, "Would have created a Pull Request with:\n")
	fmt.Fprintf(w, "title:\t%s\n", params["title"])
	fmt.Fprintf(w, "draft:\t%t\n", params["draft"])
	fmt.Fprintf(w, "base:\t%s\n", params["baseRefName"])
	fmt.Fprintf(w, "head:\t%s\n", params["headRefName"])
	if len(state.Labels) != 0 {
		fmt.Fprintf(w, "labels:\t%v\n", strings.Join(state.Labels, ", "))
	}
	if len(state.Reviewers) != 0 {
		fmt.Fprintf(w, "reviewers:\t%v\n", strings.Join(state.Reviewers, ", "))
	}
	if len(state.Assignees) != 0 {
		fmt.Fprintf(w, "assignees:\t%v\n", strings.Join(state.Assignees, ", "))
	}
	if len(state.Milestones) != 0 {
		fmt.Fprintf(w, "milestones:\t%v\n", strings.Join(state.Milestones, ", "))
	}
	if len(state.ProjectTitles) != 0 {
		fmt.Fprintf(w, "projects:\t%v\n", strings.Join(state.ProjectTitles, ", "))
	}
	fmt.Fprintf(w, "maintainerCanModify:\t%t\n", params["maintainerCanModify"])
	fmt.Fprint(w, "body:\n")
	if len(params["body"].(string)) != 0 {
		fmt.Fprintln(w, params["body"])
	}
	return nil
}

func renderPullRequestTTY(io *iostreams.IOStreams, params map[string]interface{}, state *shared.IssueMetadataState) error {
	cs := io.ColorScheme()
	out := io.Out

	fmt.Fprint(out, "Would have created a Pull Request with:\n")
	fmt.Fprintf(out, "%s: %s\n", cs.Bold("Title"), params["title"].(string))
	fmt.Fprintf(out, "%s: %t\n", cs.Bold("Draft"), params["draft"])
	fmt.Fprintf(out, "%s: %s\n", cs.Bold("Base"), params["baseRefName"])
	fmt.Fprintf(out, "%s: %s\n", cs.Bold("Head"), params["headRefName"])
	if len(state.Labels) != 0 {
		fmt.Fprintf(out, "%s: %s\n", cs.Bold("Labels"), strings.Join(state.Labels, ", "))
	}
	if len(state.Reviewers) != 0 {
		fmt.Fprintf(out, "%s: %s\n", cs.Bold("Reviewers"), strings.Join(state.Reviewers, ", "))
	}
	if len(state.Assignees) != 0 {
		fmt.Fprintf(out, "%s: %s\n", cs.Bold("Assignees"), strings.Join(state.Assignees, ", "))
	}
	if len(state.Milestones) != 0 {
		fmt.Fprintf(out, "%s: %s\n", cs.Bold("Milestones"), strings.Join(state.Milestones, ", "))
	}
	if len(state.ProjectTitles) != 0 {
		fmt.Fprintf(out, "%s: %s\n", cs.Bold("Projects"), strings.Join(state.ProjectTitles, ", "))
	}
	fmt.Fprintf(out, "%s: %t\n", cs.Bold("MaintainerCanModify"), params["maintainerCanModify"])

	fmt.Fprintf(out, "%s\n", cs.Bold("Body:"))
	// Body
	var md string
	var err error
	if len(params["body"].(string)) == 0 {
		md = fmt.Sprintf("%s\n", cs.Muted("No description provided"))
	} else {
		md, err = markdown.Render(params["body"].(string),
			markdown.WithTheme(io.TerminalTheme()),
			markdown.WithWrap(io.TerminalWidth()))
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "%s", md)

	return nil
}

func previewPR(opts CreateOptions, openURL string) error {
	if opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
	}
	return opts.Browser.Browse(openURL)
}

func handlePush(opts CreateOptions, ctx CreateContext) error {
	refs := ctx.PRRefs
	forkableRefs, requiresFork := refs.(forkableRefs)
	if requiresFork {
		opts.IO.StartProgressIndicator()
		forkedRepo, err := api.ForkRepo(ctx.Client, forkableRefs.BaseRepo(), "", "", false)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}

		refs = pushableRefs{
			headRepo:       forkedRepo,
			headBranchName: forkableRefs.qualifiedHeadRef.BranchName(),
			baseRefs: baseRefs{
				baseRepo:       forkableRefs.baseRepo,
				baseBranchName: forkableRefs.baseBranchName,
			},
		}
	}

	// We may have upcast to pushableRefs on fork, or we may have been passed an instance
	// already. But if we haven't, then there's nothing more to do.
	pushableRefs, ok := refs.(pushableRefs)
	if !ok {
		return nil
	}

	// There are two cases when an existing remote for the head repo will be
	// missing (and an error will be returned):
	// 1. the head repo was just created by auto-forking;
	// 2. an existing fork was discovered by querying the API.
	// In either case, we want to add the head repo as a new git remote so we
	// can push to it. We will try to add the head repo as the "origin" remote
	// and fallback to the "fork" remote if it is unavailable. Also, if the
	// base repo is the "origin" remote we will rename it "upstream".
	headRemote, _ := ctx.ResolvedRemotes.RemoteForRepo(pushableRefs.HeadRepo())
	if headRemote == nil {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}

		remotes, err := opts.Remotes()
		if err != nil {
			return err
		}

		cloneProtocol := cfg.GitProtocol(pushableRefs.HeadRepo().RepoHost()).Value
		headRepoURL := ghrepo.FormatRemoteURL(pushableRefs.HeadRepo(), cloneProtocol)
		gitClient := ctx.GitClient
		origin, _ := remotes.FindByName("origin")
		upstreamName := "upstream"
		upstream, _ := remotes.FindByName(upstreamName)
		remoteName := "origin"

		if origin != nil {
			remoteName = "fork"
		}

		if origin != nil && upstream == nil && ghrepo.IsSame(origin, pushableRefs.BaseRepo()) {
			renameCmd, err := gitClient.Command(context.Background(), "remote", "rename", "origin", upstreamName)
			if err != nil {
				return err
			}
			if _, err = renameCmd.Output(); err != nil {
				return fmt.Errorf("error renaming origin remote: %w", err)
			}
			remoteName = "origin"
			fmt.Fprintf(opts.IO.ErrOut, "Changed %s remote to %q\n", ghrepo.FullName(pushableRefs.BaseRepo()), upstreamName)
		}

		gitRemote, err := gitClient.AddRemote(context.Background(), remoteName, headRepoURL, []string{})
		if err != nil {
			return fmt.Errorf("error adding remote: %w", err)
		}

		fmt.Fprintf(opts.IO.ErrOut, "Added %s as remote %q\n", ghrepo.FullName(pushableRefs.HeadRepo()), remoteName)

		// Only mark `upstream` remote as default if `gh pr create` created the remote.
		if requiresFork {
			err := gitClient.SetRemoteResolution(context.Background(), upstreamName, "base")
			if err != nil {
				return fmt.Errorf("error setting upstream as default: %w", err)
			}

			if opts.IO.IsStdoutTTY() {
				cs := opts.IO.ColorScheme()
				fmt.Fprintf(opts.IO.ErrOut, "%s Repository %s set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n", cs.WarningIcon(), cs.Bold(ghrepo.FullName(pushableRefs.HeadRepo())))
			}
		}

		headRemote = &ghContext.Remote{
			Remote: gitRemote,
			Repo:   pushableRefs.HeadRepo(),
		}
	}

	// automatically push the branch if it hasn't been pushed anywhere yet
	pushBranch := func() error {
		w := NewRegexpWriter(opts.IO.ErrOut, gitPushRegexp, "")
		defer w.Flush()
		ref := fmt.Sprintf("HEAD:refs/heads/%s", ctx.PRRefs.UnqualifiedHeadRef())
		bo := backoff.NewConstantBackOff(2 * time.Second)
		root := context.Background()
		return backoff.Retry(func() error {
			if err := ctx.GitClient.Push(root, headRemote.Name, ref, git.WithStderr(w)); err != nil {
				// Only retry if we have forked the repo else the push should succeed the first time.
				if requiresFork {
					fmt.Fprintf(opts.IO.ErrOut, "waiting 2 seconds before retrying...\n")
					return err
				}
				return backoff.Permanent(err)
			}
			return nil
		}, backoff.WithContext(backoff.WithMaxRetries(bo, 3), root))
	}

	return pushBranch()
}

func generateCompareURL(ctx CreateContext, state shared.IssueMetadataState) (string, error) {
	u := ghrepo.GenerateRepoURL(
		ctx.PRRefs.BaseRepo(),
		"compare/%s...%s?expand=1",
		url.PathEscape(ctx.PRRefs.BaseRef()), url.PathEscape(ctx.PRRefs.QualifiedHeadRef()))
	// TODO wm: revisit project support
	url, err := shared.WithPrAndIssueQueryParams(ctx.Client, ctx.PRRefs.BaseRepo(), u, state, gh.ProjectsV1Supported)
	if err != nil {
		return "", err
	}
	return url, nil
}

// Humanize returns a copy of the string s that replaces all instance of '-' and '_' with spaces.
func humanize(s string) string {
	replace := "_-"
	h := func(r rune) rune {
		if strings.ContainsRune(replace, r) {
			return ' '
		}
		return r
	}
	return strings.Map(h, s)
}

func requestableReviewersForCompletion(opts *CreateOptions) ([]string, error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil, err
	}

	remotes, err := getRemotes(opts)
	if err != nil {
		return nil, err
	}
	repoContext, err := ghContext.ResolveRemotesToRepos(remotes, api.NewClientFromHTTP(httpClient), opts.RepoOverride)
	if err != nil {
		return nil, err
	}
	baseRepo, err := repoContext.BaseRepo(opts.IO)
	if err != nil {
		return nil, err
	}

	return shared.RequestableReviewersForCompletion(httpClient, baseRepo)
}

var gitPushRegexp = regexp.MustCompile("^remote: (Create a pull request.*by visiting|[[:space:]]*https://.*/pull/new/).*\n?$")
