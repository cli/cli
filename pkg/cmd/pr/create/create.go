package create

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
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

			if !opts.IO.CanPrompt() && !opts.WebMode && !opts.TitleProvided && !opts.Autofill {
				return &cmdutil.FlagError{Err: errors.New("--title or --fill required when not running interactively")}
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

func createRun(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	repoContext, err := context.ResolveRemotesToRepos(remotes, client, opts.RepoOverride)
	if err != nil {
		return err
	}

	var baseRepo *api.Repository
	if br, err := repoContext.BaseRepo(opts.IO); err == nil {
		if r, ok := br.(*api.Repository); ok {
			baseRepo = r
		} else {
			// TODO: if RepoNetwork is going to be requested anyway in `repoContext.HeadRepos()`,
			// consider piggybacking on that result instead of performing a separate lookup
			var err error
			baseRepo, err = api.GitHubRepo(client, br)
			if err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("could not determine base repository: %w", err)
	}

	isPushEnabled := false
	headBranch := opts.HeadBranch
	headBranchLabel := opts.HeadBranch
	if headBranch == "" {
		headBranch, err = opts.Branch()
		if err != nil {
			return fmt.Errorf("could not determine the current branch: %w", err)
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
			return err
		}

		if len(pushableRepos) == 0 {
			pushableRepos, err = api.RepoFindForks(client, baseRepo, 3)
			if err != nil {
				return err
			}
		}

		currentLogin, err := api.CurrentLoginName(client, baseRepo.RepoHost())
		if err != nil {
			return err
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
			return err
		}

		if selectedOption < len(pushableRepos) {
			headRepo = pushableRepos[selectedOption]
			if !ghrepo.IsSame(baseRepo, headRepo) {
				headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
			}
		} else if pushOptions[selectedOption] == "Skip pushing the branch" {
			isPushEnabled = false
		} else if pushOptions[selectedOption] == "Cancel" {
			return cmdutil.SilentError
		} else {
			// "Create a fork of ..."
			if baseRepo.IsPrivate {
				return fmt.Errorf("cannot fork private repository %s", ghrepo.FullName(baseRepo))
			}
			headBranchLabel = fmt.Sprintf("%s:%s", currentLogin, headBranch)
		}
	}

	if headRepo == nil && isPushEnabled && !opts.IO.CanPrompt() {
		fmt.Fprintf(opts.IO.ErrOut, "aborted: you must first push the current branch to a remote, or use the --head flag")
		return cmdutil.SilentError
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}
	if headBranch == baseBranch && headRepo != nil && ghrepo.IsSame(baseRepo, headRepo) {
		return fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	var milestoneTitles []string
	if opts.Milestone != "" {
		milestoneTitles = []string{opts.Milestone}
	}

	baseTrackingBranch := baseBranch
	if baseRemote, err := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName()); err == nil {
		baseTrackingBranch = fmt.Sprintf("%s/%s", baseRemote.Name, baseBranch)
	}
	defs, defaultsErr := computeDefaults(baseTrackingBranch, headBranch)

	title := opts.Title
	body := opts.Body

	action := shared.SubmitAction
	if opts.WebMode {
		action = shared.PreviewAction
		if (title == "" || body == "") && defaultsErr != nil {
			return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
		}
	} else if opts.Autofill {
		if defaultsErr != nil && !(opts.TitleProvided || opts.BodyProvided) {
			return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
		}
		if !opts.TitleProvided {
			title = defs.Title
		}
		if !opts.BodyProvided {
			body = defs.Body
		}
	}

	if !opts.WebMode {
		existingPR, err := api.PullRequestForBranch(client, baseRepo, baseBranch, headBranchLabel, []string{"OPEN"})
		var notFound *api.NotFoundError
		if err != nil && !errors.As(err, &notFound) {
			return fmt.Errorf("error checking for existing pull request: %w", err)
		}
		if err == nil {
			return fmt.Errorf("a pull request for branch %q into branch %q already exists:\n%s", headBranchLabel, baseBranch, existingPR.URL)
		}
	}

	isTerminal := opts.IO.IsStdinTTY() && opts.IO.IsStdoutTTY()

	if !opts.WebMode && !opts.Autofill {
		message := "\nCreating pull request for %s into %s in %s\n\n"
		if opts.IsDraft {
			message = "\nCreating draft pull request for %s into %s in %s\n\n"
		}

		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, message,
				utils.Cyan(headBranchLabel),
				utils.Cyan(baseBranch),
				ghrepo.FullName(baseRepo))
			if (title == "" || body == "") && defaultsErr != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s warning: could not compute title or body defaults: %s\n", utils.Yellow("!"), defaultsErr)
			}
		}
	}

	tb := shared.IssueMetadataState{
		Type:       shared.PRMetadata,
		Reviewers:  opts.Reviewers,
		Assignees:  opts.Assignees,
		Labels:     opts.Labels,
		Projects:   opts.Projects,
		Milestones: milestoneTitles,
	}

	if !opts.WebMode && !opts.Autofill && opts.Interactive {
		var nonLegacyTemplateFiles []string
		var legacyTemplateFile *string

		if opts.RootDirOverride != "" {
			nonLegacyTemplateFiles = githubtemplate.FindNonLegacy(opts.RootDirOverride, "PULL_REQUEST_TEMPLATE")
			legacyTemplateFile = githubtemplate.FindLegacy(opts.RootDirOverride, "PULL_REQUEST_TEMPLATE")
		} else if rootDir, err := git.ToplevelDir(); err == nil {
			nonLegacyTemplateFiles = githubtemplate.FindNonLegacy(rootDir, "PULL_REQUEST_TEMPLATE")
			legacyTemplateFile = githubtemplate.FindLegacy(rootDir, "PULL_REQUEST_TEMPLATE")
		}

		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}

		err = shared.TitleBodySurvey(opts.IO, editorCommand, &tb, client, baseRepo, title, body, defs, nonLegacyTemplateFiles, legacyTemplateFile, true, baseRepo.ViewerCanTriage())
		if err != nil {
			return fmt.Errorf("could not collect title and/or body: %w", err)
		}

		action = tb.Action

		if action == shared.CancelAction {
			fmt.Fprintln(opts.IO.ErrOut, "Discarding.")
			return nil
		}

		if title == "" {
			title = tb.Title
		}
		if body == "" {
			body = tb.Body
		}
	}

	if action == shared.SubmitAction && title == "" {
		return errors.New("pull request title must not be blank")
	}

	didForkRepo := false
	// if a head repository could not be determined so far, automatically create
	// one by forking the base repository
	if headRepo == nil && isPushEnabled {
		headRepo, err = api.ForkRepo(client, baseRepo)
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}
		didForkRepo = true
	}

	if headRemote == nil && headRepo != nil {
		headRemote, _ = repoContext.RemoteForRepo(headRepo)
	}

	// There are two cases when an existing remote for the head repo will be
	// missing:
	// 1. the head repo was just created by auto-forking;
	// 2. an existing fork was discovered by querying the API.
	//
	// In either case, we want to add the head repo as a new git remote so we
	// can push to it.
	if headRemote == nil && isPushEnabled {
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
	if isPushEnabled {
		pushTries := 0
		maxPushTries := 3
		for {
			if err := git.Push(headRemote.Name, fmt.Sprintf("HEAD:%s", headBranch)); err != nil {
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
	}

	if action == shared.SubmitAction {
		params := map[string]interface{}{
			"title":       title,
			"body":        body,
			"draft":       opts.IsDraft,
			"baseRefName": baseBranch,
			"headRefName": headBranchLabel,
		}

		err = shared.AddMetadataToIssueParams(client, baseRepo, params, &tb)
		if err != nil {
			return err
		}

		pr, err := api.CreatePullRequest(client, baseRepo, params)
		if pr != nil {
			fmt.Fprintln(opts.IO.Out, pr.URL)
		}
		if err != nil {
			if pr != nil {
				return fmt.Errorf("pull request update failed: %w", err)
			}
			return fmt.Errorf("pull request create failed: %w", err)
		}
	} else if action == shared.PreviewAction {
		openURL, err := generateCompareURL(baseRepo, baseBranch, headBranchLabel, title, body, tb.Assignees, tb.Labels, tb.Projects, tb.Milestones)
		if err != nil {
			return err
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	} else {
		panic("Unreachable state")
	}

	return nil
}

func computeDefaults(baseRef, headRef string) (shared.Defaults, error) {
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

func generateCompareURL(r ghrepo.Interface, base, head, title, body string, assignees, labels, projects []string, milestones []string) (string, error) {
	u := ghrepo.GenerateRepoURL(r, "compare/%s...%s?expand=1", url.QueryEscape(base), url.QueryEscape(head))
	url, err := shared.WithPrAndIssueQueryParams(u, title, body, assignees, labels, projects, milestones)
	if err != nil {
		return "", err
	}
	return url, nil
}
