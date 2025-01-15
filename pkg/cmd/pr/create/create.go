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

type CreateContext struct {
	// This struct stores contextual data about the creation process and is for building up enough
	// data to create a pull request
	RepoContext        *ghContext.ResolvedRemotes
	BaseRepo           *api.Repository
	HeadRepo           ghrepo.Interface
	BaseTrackingBranch string
	BaseBranch         string
	HeadBranch         string
	HeadBranchLabel    string
	HeadRemote         *ghContext.Remote
	IsPushEnabled      bool
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
		Selector:   ctx.HeadBranchLabel,
		BaseBranch: ctx.BaseBranch,
		States:     []string{"OPEN"},
		Fields:     []string{"url"},
	})
	var notFound *shared.NotFoundError
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("error checking for existing pull request: %w", err)
	}
	if err == nil {
		return fmt.Errorf("a pull request for branch %q into branch %q already exists:\n%s",
			ctx.HeadBranchLabel, ctx.BaseBranch, existingPR.URL)
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
			cs.Cyan(ctx.HeadBranchLabel),
			cs.Cyan(ctx.BaseBranch),
			ghrepo.FullName(ctx.BaseRepo))
	}

	if !opts.EditorMode && (opts.FillVerbose || opts.Autofill || opts.FillFirst || (opts.TitleProvided && opts.BodyProvided)) {
		err = handlePush(*opts, *ctx)
		if err != nil {
			return err
		}
		return submitPR(*opts, *ctx, *state)
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

	tpl := shared.NewTemplateManager(client.HTTP(), ctx.BaseRepo, opts.Prompter, opts.RootDirOverride, opts.RepoOverride == "", true)

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
		allowMetadata := ctx.BaseRepo.ViewerCanTriage()
		action, err = shared.ConfirmPRSubmission(opts.Prompter, allowPreview, allowMetadata, state.Draft)
		if err != nil {
			return fmt.Errorf("unable to confirm: %w", err)
		}

		if action == shared.MetadataAction {
			fetcher := &shared.MetadataFetcher{
				IO:        opts.IO,
				APIClient: client,
				Repo:      ctx.BaseRepo,
				State:     state,
			}
			err = shared.MetadataSurvey(opts.Prompter, opts.IO, ctx.BaseRepo, fetcher, state)
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
		return submitPR(*opts, *ctx, *state)
	}

	if action == shared.SubmitAction {
		return submitPR(*opts, *ctx, *state)
	}

	err = errors.New("expected to cancel, preview, or submit")
	return err
}

var regexPattern = regexp.MustCompile(`(?m)^`)

func initDefaultTitleBody(ctx CreateContext, state *shared.IssueMetadataState, useFirstCommit bool, addBody bool) error {
	baseRef := ctx.BaseTrackingBranch
	headRef := ctx.HeadBranch
	gitClient := ctx.GitClient

	commits, err := gitClient.Commits(context.Background(), baseRef, headRef)
	if err != nil {
		return err
	}

	if len(commits) == 1 || useFirstCommit {
		state.Title = commits[len(commits)-1].Title
		state.Body = commits[len(commits)-1].Body
	} else {
		state.Title = humanize(headRef)
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

// trackingRef represents a ref for a remote tracking branch.
type trackingRef struct {
	remoteName string
	branchName string
}

func (r trackingRef) String() string {
	return "refs/remotes/" + r.remoteName + "/" + r.branchName
}

func mustParseTrackingRef(text string) trackingRef {
	parts := strings.SplitN(string(text), "/", 4)
	// The only place this is called is tryDetermineTrackingRef, where we are reconstructing
	// the same tracking ref we passed in. If it doesn't match the expected format, this is a
	// programmer error we want to know about, so it's ok to panic.
	if len(parts) != 4 {
		panic(fmt.Errorf("tracking ref should have four parts: %s", text))
	}
	if parts[0] != "refs" || parts[1] != "remotes" {
		panic(fmt.Errorf("tracking ref should start with refs/remotes/: %s", text))
	}

	return trackingRef{
		remoteName: parts[2],
		branchName: parts[3],
	}
}

// tryDetermineTrackingRef is intended to try and find a remote branch on the same commit as the currently checked out
// HEAD, i.e. the local branch. If there are multiple branches that might match, the first remote is chosen, which in
// practice is determined by the sorting algorithm applied much earlier in the process, roughly "upstream", "github", "origin",
// and then everything else unstably sorted.
func tryDetermineTrackingRef(gitClient *git.Client, remotes ghContext.Remotes, localBranchName string, headBranchConfig git.BranchConfig) (trackingRef, bool) {
	// To try and determine the tracking ref for a local branch, we first construct a collection of refs
	// that might be tracking, given the current branch's config, and the list of known remotes.
	refsForLookup := []string{"HEAD"}
	if headBranchConfig.RemoteName != "" && headBranchConfig.MergeRef != "" {
		tr := trackingRef{
			remoteName: headBranchConfig.RemoteName,
			branchName: strings.TrimPrefix(headBranchConfig.MergeRef, "refs/heads/"),
		}
		refsForLookup = append(refsForLookup, tr.String())
	}

	for _, remote := range remotes {
		tr := trackingRef{
			remoteName: remote.Name,
			branchName: localBranchName,
		}
		refsForLookup = append(refsForLookup, tr.String())
	}

	// Then we ask git for details about these refs, for example, refs/remotes/origin/trunk might return a hash
	// for the remote tracking branch, trunk, for the remote, origin. If there is no ref, the git client returns
	// no ref information.
	//
	// We also first check for the HEAD ref, so that we have the hash of the currently checked out commit.
	resolvedRefs, _ := gitClient.ShowRefs(context.Background(), refsForLookup)

	// If there is more than one resolved ref, that means that at least one ref was found in addition to the HEAD.
	if len(resolvedRefs) > 1 {
		headRef := resolvedRefs[0]
		for _, r := range resolvedRefs[1:] {
			// If the hash of the remote ref doesn't match the hash of HEAD then the remote branch is not in the same
			// state, so it can't be used.
			if r.Hash != headRef.Hash {
				continue
			}
			// Otherwise we can parse the returned ref into a tracking ref and return that
			return mustParseTrackingRef(r.Name), true
		}
	}

	return trackingRef{}, false
}

func NewIssueState(ctx CreateContext, opts CreateOptions) (*shared.IssueMetadataState, error) {
	var milestoneTitles []string
	if opts.Milestone != "" {
		milestoneTitles = []string{opts.Milestone}
	}

	meReplacer := shared.NewMeReplacer(ctx.Client, ctx.BaseRepo.RepoHost())
	assignees, err := meReplacer.ReplaceSlice(opts.Assignees)
	if err != nil {
		return nil, err
	}

	state := &shared.IssueMetadataState{
		Type:       shared.PRMetadata,
		Reviewers:  opts.Reviewers,
		Assignees:  assignees,
		Labels:     opts.Labels,
		Projects:   opts.Projects,
		Milestones: milestoneTitles,
		Draft:      opts.IsDraft,
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
	repoContext, err := ghContext.ResolveRemotesToRepos(remotes, client, opts.RepoOverride)
	if err != nil {
		return nil, err
	}

	var baseRepo *api.Repository
	if br, err := repoContext.BaseRepo(opts.IO); err == nil {
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

	isPushEnabled := false
	headBranch := opts.HeadBranch
	headBranchLabel := opts.HeadBranch
	if headBranch == "" {
		headBranch, err = opts.Branch()
		if err != nil {
			return nil, fmt.Errorf("could not determine the current branch: %w", err)
		}
		headBranchLabel = headBranch
		isPushEnabled = true
	} else if idx := strings.IndexRune(headBranch, ':'); idx >= 0 {
		headBranch = headBranch[idx+1:]
	}

	gitClient := opts.GitClient
	if ucc, err := gitClient.UncommittedChangeCount(context.Background()); err == nil && ucc > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "Warning: %s\n", text.Pluralize(ucc, "uncommitted change"))
	}

	var headRepo ghrepo.Interface
	var headRemote *ghContext.Remote

	headBranchConfig, err := gitClient.ReadBranchConfig(context.Background(), headBranch)
	if err != nil {
		return nil, err
	}
	if isPushEnabled {
		// determine whether the head branch is already pushed to a remote
		if trackingRef, found := tryDetermineTrackingRef(gitClient, remotes, headBranch, headBranchConfig); found {
			isPushEnabled = false
			if r, err := remotes.FindByName(trackingRef.remoteName); err == nil {
				headRepo = r
				headRemote = r
				headBranchLabel = trackingRef.branchName
				if !ghrepo.IsSame(baseRepo, headRepo) {
					headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), trackingRef.branchName)
				}
			}
		}
	}

	// otherwise, ask the user for the head repository using info obtained from the API
	if headRepo == nil && isPushEnabled && opts.IO.CanPrompt() {
		pushableRepos, err := repoContext.HeadRepos()
		if err != nil {
			return nil, err
		}

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
			pushOptions = append(pushOptions, "Create a fork of "+ghrepo.FullName(baseRepo))
		}
		pushOptions = append(pushOptions, "Skip pushing the branch")
		pushOptions = append(pushOptions, "Cancel")

		selectedOption, err := opts.Prompter.Select(fmt.Sprintf("Where should we push the '%s' branch?", headBranch), "", pushOptions)
		if err != nil {
			return nil, err
		}

		if selectedOption < len(pushableRepos) {
			headRepo = pushableRepos[selectedOption]
			if !ghrepo.IsSame(baseRepo, headRepo) {
				headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
			}
		} else if pushOptions[selectedOption] == "Skip pushing the branch" {
			isPushEnabled = false
		} else if pushOptions[selectedOption] == "Cancel" {
			return nil, cmdutil.CancelError
		} else {
			// "Create a fork of ..."
			headBranchLabel = fmt.Sprintf("%s:%s", currentLogin, headBranch)
		}
	}

	if headRepo == nil && isPushEnabled && !opts.IO.CanPrompt() {
		fmt.Fprintf(opts.IO.ErrOut, "aborted: you must first push the current branch to a remote, or use the --head flag")
		return nil, cmdutil.SilentError
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = headBranchConfig.MergeBase
	}
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}
	if headBranch == baseBranch && headRepo != nil && ghrepo.IsSame(baseRepo, headRepo) {
		return nil, fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	baseTrackingBranch := baseBranch
	if baseRemote, err := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName()); err == nil {
		baseTrackingBranch = fmt.Sprintf("%s/%s", baseRemote.Name, baseBranch)
	}

	return &CreateContext{
		BaseRepo:           baseRepo,
		HeadRepo:           headRepo,
		BaseBranch:         baseBranch,
		BaseTrackingBranch: baseTrackingBranch,
		HeadBranch:         headBranch,
		HeadBranchLabel:    headBranchLabel,
		HeadRemote:         headRemote,
		IsPushEnabled:      isPushEnabled,
		RepoContext:        repoContext,
		Client:             client,
		GitClient:          gitClient,
	}, nil
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

func submitPR(opts CreateOptions, ctx CreateContext, state shared.IssueMetadataState) error {
	client := ctx.Client

	params := map[string]interface{}{
		"title":               state.Title,
		"body":                state.Body,
		"draft":               state.Draft,
		"baseRefName":         ctx.BaseBranch,
		"headRefName":         ctx.HeadBranchLabel,
		"maintainerCanModify": opts.MaintainerCanModify,
	}

	if params["title"] == "" {
		return errors.New("pull request title must not be blank")
	}

	err := shared.AddMetadataToIssueParams(client, ctx.BaseRepo, params, &state)
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
	pr, err := api.CreatePullRequest(client, ctx.BaseRepo, params)
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
	if len(state.Projects) != 0 {
		fmt.Fprintf(w, "projects:\t%v\n", strings.Join(state.Projects, ", "))
	}
	fmt.Fprintf(w, "maintainerCanModify:\t%t\n", params["maintainerCanModify"])
	fmt.Fprint(w, "body:\n")
	if len(params["body"].(string)) != 0 {
		fmt.Fprintln(w, params["body"])
	}
	return nil
}

func renderPullRequestTTY(io *iostreams.IOStreams, params map[string]interface{}, state *shared.IssueMetadataState) error {
	iofmt := io.ColorScheme()
	out := io.Out

	fmt.Fprint(out, "Would have created a Pull Request with:\n")
	fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Title"), params["title"].(string))
	fmt.Fprintf(out, "%s: %t\n", iofmt.Bold("Draft"), params["draft"])
	fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Base"), params["baseRefName"])
	fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Head"), params["headRefName"])
	if len(state.Labels) != 0 {
		fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Labels"), strings.Join(state.Labels, ", "))
	}
	if len(state.Reviewers) != 0 {
		fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Reviewers"), strings.Join(state.Reviewers, ", "))
	}
	if len(state.Assignees) != 0 {
		fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Assignees"), strings.Join(state.Assignees, ", "))
	}
	if len(state.Milestones) != 0 {
		fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Milestones"), strings.Join(state.Milestones, ", "))
	}
	if len(state.Projects) != 0 {
		fmt.Fprintf(out, "%s: %s\n", iofmt.Bold("Projects"), strings.Join(state.Projects, ", "))
	}
	fmt.Fprintf(out, "%s: %t\n", iofmt.Bold("MaintainerCanModify"), params["maintainerCanModify"])

	fmt.Fprintf(out, "%s\n", iofmt.Bold("Body:"))
	// Body
	var md string
	var err error
	if len(params["body"].(string)) == 0 {
		md = fmt.Sprintf("%s\n", iofmt.Gray("No description provided"))
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
	didForkRepo := false
	headRepo := ctx.HeadRepo
	headRemote := ctx.HeadRemote
	client := ctx.Client

	var err error
	// if a head repository could not be determined so far, automatically create
	// one by forking the base repository
	if headRepo == nil && ctx.IsPushEnabled {
		opts.IO.StartProgressIndicator()
		headRepo, err = api.ForkRepo(client, ctx.BaseRepo, "", "", false)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}
		didForkRepo = true
	}

	if headRemote == nil && headRepo != nil {
		headRemote, _ = ctx.RepoContext.RemoteForRepo(headRepo)
	}

	// There are two cases when an existing remote for the head repo will be
	// missing:
	// 1. the head repo was just created by auto-forking;
	// 2. an existing fork was discovered by querying the API.
	// In either case, we want to add the head repo as a new git remote so we
	// can push to it. We will try to add the head repo as the "origin" remote
	// and fallback to the "fork" remote if it is unavailable. Also, if the
	// base repo is the "origin" remote we will rename it "upstream".
	if headRemote == nil && ctx.IsPushEnabled {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}

		remotes, err := opts.Remotes()
		if err != nil {
			return err
		}

		cloneProtocol := cfg.GitProtocol(headRepo.RepoHost()).Value
		headRepoURL := ghrepo.FormatRemoteURL(headRepo, cloneProtocol)
		gitClient := ctx.GitClient
		origin, _ := remotes.FindByName("origin")
		upstream, _ := remotes.FindByName("upstream")
		remoteName := "origin"

		if origin != nil {
			remoteName = "fork"
		}

		if origin != nil && upstream == nil && ghrepo.IsSame(origin, ctx.BaseRepo) {
			renameCmd, err := gitClient.Command(context.Background(), "remote", "rename", "origin", "upstream")
			if err != nil {
				return err
			}
			if _, err = renameCmd.Output(); err != nil {
				return fmt.Errorf("error renaming origin remote: %w", err)
			}
			remoteName = "origin"
			fmt.Fprintf(opts.IO.ErrOut, "Changed %s remote to %q\n", ghrepo.FullName(ctx.BaseRepo), "upstream")
		}

		gitRemote, err := gitClient.AddRemote(context.Background(), remoteName, headRepoURL, []string{})
		if err != nil {
			return fmt.Errorf("error adding remote: %w", err)
		}

		fmt.Fprintf(opts.IO.ErrOut, "Added %s as remote %q\n", ghrepo.FullName(headRepo), remoteName)

		headRemote = &ghContext.Remote{
			Remote: gitRemote,
			Repo:   headRepo,
		}
	}

	// automatically push the branch if it hasn't been pushed anywhere yet
	if ctx.IsPushEnabled {
		pushBranch := func() error {
			w := NewRegexpWriter(opts.IO.ErrOut, gitPushRegexp, "")
			defer w.Flush()
			gitClient := ctx.GitClient
			ref := fmt.Sprintf("HEAD:refs/heads/%s", ctx.HeadBranch)
			bo := backoff.NewConstantBackOff(2 * time.Second)
			ctx := context.Background()
			return backoff.Retry(func() error {
				if err := gitClient.Push(ctx, headRemote.Name, ref, git.WithStderr(w)); err != nil {
					// Only retry if we have forked the repo else the push should succeed the first time.
					if didForkRepo {
						fmt.Fprintf(opts.IO.ErrOut, "waiting 2 seconds before retrying...\n")
						return err
					}
					return backoff.Permanent(err)
				}
				return nil
			}, backoff.WithContext(backoff.WithMaxRetries(bo, 3), ctx))
		}

		err := pushBranch()
		if err != nil {
			return err
		}
	}

	return nil
}

func generateCompareURL(ctx CreateContext, state shared.IssueMetadataState) (string, error) {
	u := ghrepo.GenerateRepoURL(
		ctx.BaseRepo,
		"compare/%s...%s?expand=1",
		url.PathEscape(ctx.BaseBranch), url.PathEscape(ctx.HeadBranchLabel))
	url, err := shared.WithPrAndIssueQueryParams(ctx.Client, ctx.BaseRepo, u, state)
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
