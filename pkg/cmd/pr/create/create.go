package create

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	RootDirOverride string

	RepoOverride string

	Autofill bool
	WebMode  bool

	IsDraft       bool
	Title         string
	TitleProvided bool
	Body          string
	BodyProvided  bool
	BaseBranch    string

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
		Example: heredoc.Doc(`
			$ gh pr create --title "The bug is fixed" --body "Everything works again"
			$ gh issue create --label "bug,help wanted"
			$ gh issue create --label bug --label "help wanted"
			$ gh pr create --reviewer monalisa,hubot
			$ gh pr create --project "Roadmap"
			$ gh pr create --base develop
    	`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.TitleProvided = cmd.Flags().Changed("title")
			opts.BodyProvided = cmd.Flags().Changed("body")
			opts.RepoOverride, _ = cmd.Flags().GetString("repo")

			if !opts.IO.CanPrompt() && !opts.WebMode && !opts.TitleProvided && !opts.Autofill {
				return errors.New("--title or --fill required when prompts are disabled")
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
	fl.StringVarP(&opts.Title, "title", "t", "", "Supply a title. Will prompt for one otherwise.")
	fl.StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	fl.StringVarP(&opts.BaseBranch, "base", "B", "", "The branch into which you want your code merged")
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

	baseRepo, err := repoContext.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repository: %w", err)
	}

	headBranch, err := opts.Branch()
	if err != nil {
		return fmt.Errorf("could not determine the current branch: %w", err)
	}

	var headRepo ghrepo.Interface
	var headRemote *context.Remote

	// determine whether the head branch is already pushed to a remote
	headBranchPushedTo := determineTrackingBranch(remotes, headBranch)
	if headBranchPushedTo != nil {
		for _, r := range remotes {
			if r.Name != headBranchPushedTo.RemoteName {
				continue
			}
			headRepo = r
			headRemote = r
			break
		}
	}

	// otherwise, determine the head repository with info obtained from the API
	if headRepo == nil {
		if r, err := repoContext.HeadRepo(); err == nil {
			headRepo = r
		}
	}

	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = baseRepo.DefaultBranchRef.Name
	}
	if headBranch == baseBranch && headRepo != nil && ghrepo.IsSame(baseRepo, headRepo) {
		return fmt.Errorf("must be on a branch named differently than %q", baseBranch)
	}

	if ucc, err := git.UncommittedChangeCount(); err == nil && ucc > 0 {
		fmt.Fprintf(opts.IO.ErrOut, "Warning: %s\n", utils.Pluralize(ucc, "uncommitted change"))
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
		if defaultsErr != nil {
			return fmt.Errorf("could not compute title or body defaults: %w", defaultsErr)
		}
		title = defs.Title
		body = defs.Body
	}

	if !opts.WebMode {
		headBranchLabel := headBranch
		if headRepo != nil && !ghrepo.IsSame(baseRepo, headRepo) {
			headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
		}
		existingPR, err := api.PullRequestForBranch(client, baseRepo, baseBranch, headBranchLabel)
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
				utils.Cyan(headBranch),
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

	interactive := isTerminal && !(opts.TitleProvided && opts.BodyProvided)

	if !opts.WebMode && !opts.Autofill && interactive {
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
	if headRepo == nil {
		if baseRepo.IsPrivate {
			return fmt.Errorf("cannot fork private repository '%s'", ghrepo.FullName(baseRepo))
		}
		headRepo, err = api.ForkRepo(client, baseRepo)
		if err != nil {
			return fmt.Errorf("error forking repo: %w", err)
		}
		didForkRepo = true
	}

	headBranchLabel := headBranch
	if !ghrepo.IsSame(baseRepo, headRepo) {
		headBranchLabel = fmt.Sprintf("%s:%s", headRepo.RepoOwner(), headBranch)
	}

	if headRemote == nil {
		headRemote, _ = repoContext.RemoteForRepo(headRepo)
	}

	// There are two cases when an existing remote for the head repo will be
	// missing:
	// 1. the head repo was just created by auto-forking;
	// 2. an existing fork was discovered by quering the API.
	//
	// In either case, we want to add the head repo as a new git remote so we
	// can push to it.
	if headRemote == nil {
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
	if headBranchPushedTo == nil {
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

		body := ""
		for i := len(commits) - 1; i >= 0; i-- {
			body += fmt.Sprintf("- %s\n", commits[i].Title)
		}
		out.Body = body
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
	u := ghrepo.GenerateRepoURL(r, "compare/%s...%s?expand=1", base, head)
	url, err := shared.WithPrAndIssueQueryParams(u, title, body, assignees, labels, projects, milestones)
	if err != nil {
		return "", err
	}
	return url, nil
}
