package create

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	// This struct stores user input and factory functions
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	Interactive bool

	TitleProvided bool
	BodyProvided  bool

	RootDirOverride string
	RepoOverride    string

	Autofill bool
	WebMode  bool

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
}

type CreateContext struct {
	// This struct stores contextual data about the creation process and is for building up enough
	// data to create a pull request
	RepoContext        *context.ResolvedRemotes
	BaseRepo           *api.Repository
	HeadRepo           ghrepo.Interface
	BaseTrackingBranch string
	BaseBranch         string
	HeadBranch         string
	HeadBranchLabel    string
	HeadRemote         *context.Remote
	IsPushEnabled      bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: heredoc.Doc(`
			Create a pull request on GitHub.

			When the current branch isn't fully pushed to a git remote, a prompt will ask where
			to push the branch and offer an option to fork the base repository. Use '--head' to
			explicitly skip any forking or pushing behavior.

			A prompt will also ask for the title and the body of the pull request. Use '--title'
			and '--body' to skip this, or use '--fill' to autofill these values from git commits.
		`),
		Example: heredoc.Doc(`
			$ gh pr create --title "The bug is fixed" --body "Everything works again"
			$ gh pr create --reviewer monalisa,hubot
			$ gh pr create --project "Roadmap"
			$ gh pr create --base develop --head monalisa:feature
		`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.TitleProvided = cmd.Flags().Changed("title")
			opts.BodyProvided = cmd.Flags().Changed("body")
			opts.RepoOverride, _ = cmd.Flags().GetString("repo")

			opts.Interactive = !(opts.TitleProvided && opts.BodyProvided)

			// TODO check on edge cases around title/body provision

			if !opts.IO.CanPrompt() && !opts.WebMode && !opts.TitleProvided && !opts.Autofill {
				return &cmdutil.FlagError{Err: errors.New("--title or --fill required when not running interactively")}
			}

			if !opts.IO.CanPrompt() {
				opts.Interactive = false
			}

			if opts.IsDraft && opts.WebMode {
				return errors.New("the --draft flag is not supported with --web")
			}
			if len(opts.Reviewers) > 0 && opts.WebMode {
				return errors.New("the --reviewer flag is not supported with --web")
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
	fl.StringVarP(&opts.BaseBranch, "base", "B", "", "The `branch` into which you want your code merged")
	fl.StringVarP(&opts.HeadBranch, "head", "H", "", "The `branch` that contains commits for your pull request (default: current branch)")
	fl.BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser to create a pull request")
	fl.BoolVarP(&opts.Autofill, "fill", "f", false, "Do not prompt for title/body and just use commit info")
	fl.StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people by their `login`")
	fl.StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their `login`")
	fl.StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by `name`")
	fl.StringSliceVarP(&opts.Projects, "project", "p", nil, "Add the pull request to projects by `name`")
	fl.StringVarP(&opts.Milestone, "milestone", "m", "", "Add the pull request to a milestone by `name`")

	return cmd
}

func createRun(opts *CreateOptions) (err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	ctx, err := NewCreateContext(opts)
	if err != nil {
		return err
	}

	defs, defaultsErr := computeDefaults(*ctx)
	if defaultsErr != nil && (opts.Autofill || opts.WebMode || !opts.Interactive) {
		return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
	}

	var milestoneTitles []string
	if opts.Milestone != "" {
		milestoneTitles = []string{opts.Milestone}
	}

	state := shared.IssueMetadataState{
		Title:      defs.Title,
		Body:       defs.Body,
		Type:       shared.PRMetadata,
		Reviewers:  opts.Reviewers,
		Assignees:  opts.Assignees,
		Labels:     opts.Labels,
		Projects:   opts.Projects,
		Milestones: milestoneTitles,
	}

	if opts.TitleProvided {
		state.Title = opts.Title
	}

	if opts.BodyProvided {
		state.Body = opts.Body
	}

	if opts.WebMode {
		err := handlePush(*opts, *ctx)
		if err != nil {
			return err
		}
		return previewPR(*opts, *ctx, state)
	}

	if opts.Autofill || !opts.Interactive {
		return submitPR(*opts, *ctx, state)
	}

	existingPR, err := api.PullRequestForBranch(
		client, ctx.BaseRepo, ctx.BaseBranch, ctx.HeadBranchLabel, []string{"OPEN"})
	var notFound *api.NotFoundError
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("error checking for existing pull request: %w", err)
	}
	if err == nil {
		return fmt.Errorf("a pull request for branch %q into branch %q already exists:\n%s",
			ctx.HeadBranchLabel, ctx.BaseBranch, existingPR.URL)
	}

	message := "\nCreating pull request for %s into %s in %s\n\n"
	if opts.IsDraft {
		message = "\nCreating draft pull request for %s into %s in %s\n\n"
	}

	cs := opts.IO.ColorScheme()

	fmt.Fprintf(opts.IO.ErrOut, message,
		cs.Cyan(ctx.HeadBranchLabel),
		cs.Cyan(ctx.BaseBranch),
		ghrepo.FullName(ctx.BaseRepo))
	if (state.Title == "" || state.Body == "") && defaultsErr != nil {
		fmt.Fprintf(opts.IO.ErrOut,
			"%s warning: could not compute title or body defaults: %s\n", cs.Yellow("!"), defaultsErr)
	}

	if !opts.TitleProvided {
		err = shared.TitleSurvey(&state)
		if err != nil {
			return err
		}
	}

	editorCommand, err := cmdutil.DetermineEditor(opts.Config)
	if err != nil {
		return err
	}

	templateContent := ""
	if !opts.BodyProvided {
		templateFiles, legacyTemplate := shared.FindTemplates(opts.RootDirOverride, "PULL_REQUEST_TEMPLATE")

		templateContent, err = shared.TemplateSurvey(templateFiles, legacyTemplate, state)
		if err != nil {
			return err
		}

		err = shared.BodySurvey(&state, templateContent, editorCommand)
		if err != nil {
			return err
		}

		if state.Body == "" {
			state.Body = templateContent
		}
	}

	allowMetadata := ctx.BaseRepo.ViewerCanTriage()
	action, err := shared.ConfirmSubmission(!state.HasMetadata(), allowMetadata)
	if err != nil {
		return fmt.Errorf("unable to confirm: %w", err)
	}

	if action == shared.MetadataAction {
		err = shared.MetadataSurvey(opts.IO, client, ctx.BaseRepo, &state)
		if err != nil {
			return err
		}

		action, err = shared.ConfirmSubmission(!state.HasMetadata(), false)
		if err != nil {
			return err
		}
	}

	if action == shared.CancelAction {
		fmt.Fprintln(opts.IO.ErrOut, "Discarding.")
		return nil
	}

	err = handlePush(*opts, *ctx)
	if err != nil {
		return err
	}

	if action == shared.PreviewAction {
		return previewPR(*opts, *ctx, state)
	}

	if action == shared.SubmitAction {
		return submitPR(*opts, *ctx, state)
	}

	return errors.New("expected to cancel, preview, or submit")
}

func computeDefaults(createCtx CreateContext) (shared.Defaults, error) {
	baseRef := createCtx.BaseTrackingBranch
	headRef := createCtx.HeadBranch
	out := shared.Defaults{}

	commits, err := git.Commits(baseRef, headRef)
	if err != nil {
		return out, err
	}

	if len(commits) == 1 {
		out.Title = commits[0].Title
		body, err := git.CommitBody(commits[0].Sha)
		if err != nil {
			return out, err
		}
		out.Body = body
	} else {
		out.Title = utils.Humanize(headRef)

		var body strings.Builder
		for i := len(commits) - 1; i >= 0; i-- {
			fmt.Fprintf(&body, "- %s\n", commits[i].Title)
		}
		out.Body = body.String()
	}

	return out, nil
}

func determineTrackingBranch(remotes context.Remotes, headBranch string) *git.TrackingRef {
	refsForLookup := []string{"HEAD"}
	var trackingRefs []git.TrackingRef

	headBranchConfig := git.ReadBranchConfig(headBranch)
	if headBranchConfig.RemoteName != "" {
		tr := git.TrackingRef{
			RemoteName: headBranchConfig.RemoteName,
			BranchName: strings.TrimPrefix(headBranchConfig.MergeRef, "refs/heads/"),
		}
		trackingRefs = append(trackingRefs, tr)
		refsForLookup = append(refsForLookup, tr.String())
	}

	for _, remote := range remotes {
		tr := git.TrackingRef{
			RemoteName: remote.Name,
			BranchName: headBranch,
		}
		trackingRefs = append(trackingRefs, tr)
		refsForLookup = append(refsForLookup, tr.String())
	}

	resolvedRefs, _ := git.ShowRefs(refsForLookup...)
	if len(resolvedRefs) > 1 {
		for _, r := range resolvedRefs[1:] {
			if r.Hash != resolvedRefs[0].Hash {
				continue
			}
			for _, tr := range trackingRefs {
				if tr.String() != r.Name {
					continue
				}
				return &tr
			}
		}
	}

	return nil
}

func NewCreateContext(opts *CreateOptions) (*CreateContext, error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil, err
	}
	client := api.NewClientFromHTTP(httpClient)

	remotes, err := opts.Remotes()
	if err != nil {
		return nil, err
	}

	repoContext, err := context.ResolveRemotesToRepos(remotes, client, opts.RepoOverride)
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
		return nil, fmt.Errorf("could not determine base repository: %w", err)
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

	if ucc, err := git.UncommittedChangeCount(); err == nil && ucc > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "Warning: %s\n", utils.Pluralize(ucc, "uncommitted change"))
	}

	var headRepo ghrepo.Interface
	var headRemote *context.Remote

	if isPushEnabled {
		// determine whether the head branch is already pushed to a remote
		if pushedTo := determineTrackingBranch(remotes, headBranch); pushedTo != nil {
			isPushEnabled = false
			if r, err := remotes.FindByName(pushedTo.RemoteName); err == nil {
				headRepo = r
				headRemote = r
				headBranchLabel = pushedTo.BranchName
				if !ghrepo.IsSame(baseRepo, headRepo) {
					headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), pushedTo.BranchName)
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

		var selectedOption int
		err = prompt.SurveyAskOne(&survey.Select{
			Message: fmt.Sprintf("Where should we push the '%s' branch?", headBranch),
			Options: pushOptions,
		}, &selectedOption)
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
			return nil, cmdutil.SilentError
		} else {
			// "Create a fork of ..."
			if baseRepo.IsPrivate {
				return nil, fmt.Errorf("cannot fork private repository %s", ghrepo.FullName(baseRepo))
			}
			headBranchLabel = fmt.Sprintf("%s:%s", currentLogin, headBranch)
		}
	}

	if headRepo == nil && isPushEnabled && !opts.IO.CanPrompt() {
		fmt.Fprintf(opts.IO.ErrOut, "aborted: you must first push the current branch to a remote, or use the --head flag")
		return nil, cmdutil.SilentError
	}

	baseBranch := opts.BaseBranch
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
	}, nil

}

func submitPR(opts CreateOptions, createCtx CreateContext, state shared.IssueMetadataState) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return nil
	}
	client := api.NewClientFromHTTP(httpClient)

	params := map[string]interface{}{
		"title":       state.Title,
		"body":        state.Body,
		"draft":       opts.IsDraft,
		"baseRefName": createCtx.BaseBranch,
		"headRefName": createCtx.HeadBranchLabel,
	}

	if params["title"] == "" {
		return errors.New("pull request title must not be blank")
	}

	err = shared.AddMetadataToIssueParams(client, createCtx.BaseRepo, params, &state)
	if err != nil {
		return err
	}

	pr, err := api.CreatePullRequest(client, createCtx.BaseRepo, params)
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

func previewPR(opts CreateOptions, createCtx CreateContext, state shared.IssueMetadataState) error {
	openURL, err := generateCompareURL(createCtx, state)
	if err != nil {
		return err
	}

	if opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
	}
	return utils.OpenInBrowser(openURL)

}

func handlePush(opts CreateOptions, ctx CreateContext) error {
	didForkRepo := false
	headRepo := ctx.HeadRepo
	headRemote := ctx.HeadRemote

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	// if a head repository could not be determined so far, automatically create
	// one by forking the base repository
	if headRepo == nil && ctx.IsPushEnabled {
		headRepo, err = api.ForkRepo(client, ctx.BaseRepo)
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
	//
	// In either case, we want to add the head repo as a new git remote so we
	// can push to it.
	if headRemote == nil && ctx.IsPushEnabled {
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		cloneProtocol, _ := cfg.Get(headRepo.RepoHost(), "git_protocol")

		headRepoURL := ghrepo.FormatRemoteURL(headRepo, cloneProtocol)

		// TODO: prevent clashes with another remote of a same name
		gitRemote, err := git.AddRemote("fork", headRepoURL)
		if err != nil {
			return fmt.Errorf("error adding remote: %w", err)
		}
		headRemote = &context.Remote{
			Remote: gitRemote,
			Repo:   headRepo,
		}
	}

	// automatically push the branch if it hasn't been pushed anywhere yet
	if ctx.IsPushEnabled {
		pushBranch := func() error {
			pushTries := 0
			maxPushTries := 3
			for {
				r := NewRegexpWriter(opts.IO.ErrOut, gitPushRegexp, "")
				defer r.Flush()
				cmdErr := r
				cmdOut := opts.IO.Out
				if err := git.Push(headRemote.Name, fmt.Sprintf("HEAD:%s", ctx.HeadBranch), cmdOut, cmdErr); err != nil {
					if didForkRepo && pushTries < maxPushTries {
						pushTries++
						// first wait 2 seconds after forking, then 4s, then 6s
						waitSeconds := 2 * pushTries
						fmt.Fprintf(opts.IO.ErrOut, "waiting %s before retrying...\n", utils.Pluralize(waitSeconds, "second"))
						time.Sleep(time.Duration(waitSeconds) * time.Second)
						continue
					}
					return err
				}
				break
			}
			return nil
		}

		err := pushBranch()
		if err != nil {
			return err
		}
	}

	return nil
}

func generateCompareURL(createCtx CreateContext, state shared.IssueMetadataState) (string, error) {
	u := ghrepo.GenerateRepoURL(
		createCtx.BaseRepo,
		"compare/%s...%s?expand=1",
		url.QueryEscape(createCtx.BaseBranch), url.QueryEscape(createCtx.HeadBranch))
	url, err := shared.WithPrAndIssueQueryParams(u, state)
	if err != nil {
		return "", err
	}
	return url, nil
}

var gitPushRegexp = regexp.MustCompile("^remote: (Create a pull request.*by visiting|[[:space:]]*https://.*/pull/new/).*\n?$")
