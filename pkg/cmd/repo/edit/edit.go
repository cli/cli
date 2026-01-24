package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type iprompter interface {
	MultiSelect(prompt string, defaults []string, options []string) ([]int, error)
	Input(string, string) (string, error)
	Confirm(string, bool) (bool, error)
	Select(string, string, []string) (int, error)
}

const (
	allowMergeCommits = "Allow Merge Commits"
	allowSquashMerge  = "Allow Squash Merging"
	allowRebaseMerge  = "Allow Rebase Merging"

	optionAllowForking      = "Allow Forking"
	optionCollaborators      = "Collaborators"
	optionDefaultBranchName  = "Default Branch Name"
	optionDescription        = "Description"
	optionHomePageURL        = "Home Page URL"
	optionIssues             = "Issues"
	optionMergeOptions       = "Merge Options"
	optionProjects           = "Projects"
	optionDiscussions        = "Discussions"
	optionTemplateRepo       = "Template Repository"
	optionTopics             = "Topics"
	optionVisibility         = "Visibility"
	optionWikis              = "Wikis"
)

type EditOptions struct {
	HTTPClient                         *http.Client
	Repository                         ghrepo.Interface
	IO                                 *iostreams.IOStreams
	Edits                              EditRepositoryInput
	AddTopics                          []string
	RemoveTopics                       []string
	AddCollaborators                   []string
	RemoveCollaborators                []string
	AddTeams                           []string
	RemoveTeams                        []string
	AcceptVisibilityChangeConsequences bool
	InteractiveMode                    bool
	Detector                           fd.Detector
	Prompter                           iprompter
	// Cache of current repo topics to avoid retrieving them
	// in multiple flows.
	topicsCache []string
}

type EditRepositoryInput struct {
	enableAdvancedSecurity             *bool
	enableSecretScanning               *bool
	enableSecretScanningPushProtection *bool

	AllowForking        *bool                     `json:"allow_forking,omitempty"`
	AllowUpdateBranch   *bool                     `json:"allow_update_branch,omitempty"`
	DefaultBranch       *string                   `json:"default_branch,omitempty"`
	DeleteBranchOnMerge *bool                     `json:"delete_branch_on_merge,omitempty"`
	Description         *string                   `json:"description,omitempty"`
	EnableAutoMerge     *bool                     `json:"allow_auto_merge,omitempty"`
	EnableIssues        *bool                     `json:"has_issues,omitempty"`
	EnableMergeCommit   *bool                     `json:"allow_merge_commit,omitempty"`
	EnableProjects      *bool                     `json:"has_projects,omitempty"`
	EnableDiscussions   *bool                     `json:"has_discussions,omitempty"`
	EnableRebaseMerge   *bool                     `json:"allow_rebase_merge,omitempty"`
	EnableSquashMerge   *bool                     `json:"allow_squash_merge,omitempty"`
	EnableWiki          *bool                     `json:"has_wiki,omitempty"`
	Homepage            *string                   `json:"homepage,omitempty"`
	IsTemplate          *bool                     `json:"is_template,omitempty"`
	SecurityAndAnalysis *SecurityAndAnalysisInput `json:"security_and_analysis,omitempty"`
	Visibility          *string                   `json:"visibility,omitempty"`
}

func NewCmdEdit(f *cmdutil.Factory, runF func(options *EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "edit [<repository>]",
		Short: "Edit repository settings",
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A repository can be supplied as an argument in any of the following formats:
				- "OWNER/REPO"
				- by URL, e.g. "https://github.com/OWNER/REPO"
			`),
		},
		Long: heredoc.Docf(`
			Edit repository settings.

			To toggle a setting off, use the %[1]s--<flag>=false%[1]s syntax.

			Changing repository visibility can have unexpected consequences including but not limited to:

			- Losing stars and watchers, affecting repository ranking
			- Detaching public forks from the network
			- Disabling push rulesets
			- Allowing access to GitHub Actions history and logs

			When the %[1]s--visibility%[1]s flag is used, %[1]s--accept-visibility-change-consequences%[1]s flag is required.

			For information on all the potential consequences, see <https://gh.io/setting-repository-visibility>.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# Enable issues and wiki
			$ gh repo edit --enable-issues --enable-wiki

			# Disable projects
			$ gh repo edit --enable-projects=false
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				var err error
				opts.Repository, err = ghrepo.FromFullName(args[0])
				if err != nil {
					return err
				}
			} else {
				var err error
				opts.Repository, err = f.BaseRepo()
				if err != nil {
					return err
				}
			}

			if httpClient, err := f.HttpClient(); err == nil {
				opts.HTTPClient = httpClient
			} else {
				return err
			}

			if cmd.Flags().NFlag() == 0 {
				opts.InteractiveMode = true
			}

			if opts.InteractiveMode && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("specify properties to edit when not running interactively")
			}

			if opts.Edits.Visibility != nil && !opts.AcceptVisibilityChangeConsequences {
				return cmdutil.FlagErrorf("use of --visibility flag requires --accept-visibility-change-consequences flag")
			}

			if hasSecurityEdits(opts.Edits) {
				opts.Edits.SecurityAndAnalysis = transformSecurityAndAnalysisOpts(opts)
			}

			// Validate collaborator inputs
			for _, input := range opts.AddCollaborators {
				parts := strings.SplitN(input, ":", 2)
				if len(parts) != 2 {
					return cmdutil.FlagErrorf("invalid collaborator format: %s (expected 'username:permission')", input)
				}
				permission := strings.ToLower(strings.TrimSpace(parts[1]))
				if !isValidPermission(permission) {
					return cmdutil.FlagErrorf("invalid permission: %s (must be one of: pull, push, admin, maintain, triage)", parts[1])
				}
			}

			// Validate team inputs
			for _, input := range opts.AddTeams {
				parts := strings.SplitN(input, ":", 2)
				if len(parts) != 2 {
					return cmdutil.FlagErrorf("invalid team format: %s (expected 'org/team:permission')", input)
				}
				permission := strings.ToLower(strings.TrimSpace(parts[1]))
				if !isValidPermission(permission) {
					return cmdutil.FlagErrorf("invalid permission: %s (must be one of: pull, push, admin, maintain, triage)", parts[1])
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return editRun(cmd.Context(), opts)
		},
	}

	cmdutil.NilStringFlag(cmd, &opts.Edits.Description, "description", "d", "Description of the repository")
	cmdutil.NilStringFlag(cmd, &opts.Edits.Homepage, "homepage", "h", "Repository home page `URL`")
	cmdutil.NilStringFlag(cmd, &opts.Edits.DefaultBranch, "default-branch", "", "Set the default branch `name` for the repository")
	cmdutil.NilStringFlag(cmd, &opts.Edits.Visibility, "visibility", "", "Change the visibility of the repository to {public,private,internal}")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.IsTemplate, "template", "", "Make the repository available as a template repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableIssues, "enable-issues", "", "Enable issues in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableProjects, "enable-projects", "", "Enable projects in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableWiki, "enable-wiki", "", "Enable wiki in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableDiscussions, "enable-discussions", "", "Enable discussions in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableMergeCommit, "enable-merge-commit", "", "Enable merging pull requests via merge commit")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableSquashMerge, "enable-squash-merge", "", "Enable merging pull requests via squashed commit")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableRebaseMerge, "enable-rebase-merge", "", "Enable merging pull requests via rebase")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableAutoMerge, "enable-auto-merge", "", "Enable auto-merge functionality")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.enableAdvancedSecurity, "enable-advanced-security", "", "Enable advanced security in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.enableSecretScanning, "enable-secret-scanning", "", "Enable secret scanning in the repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.enableSecretScanningPushProtection, "enable-secret-scanning-push-protection", "", "Enable secret scanning push protection in the repository. Secret scanning must be enabled first")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.DeleteBranchOnMerge, "delete-branch-on-merge", "", "Delete head branch when pull requests are merged")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.AllowForking, "allow-forking", "", "Allow forking of an organization repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.AllowUpdateBranch, "allow-update-branch", "", "Allow a pull request head branch that is behind its base branch to be updated")
	cmd.Flags().StringSliceVar(&opts.AddTopics, "add-topic", nil, "Add repository topic")
	cmd.Flags().StringSliceVar(&opts.RemoveTopics, "remove-topic", nil, "Remove repository topic")
	cmd.Flags().BoolVar(&opts.AcceptVisibilityChangeConsequences, "accept-visibility-change-consequences", false, "Accept the consequences of changing the repository visibility")
	
	// Collaborator management flags
	cmd.Flags().StringSliceVarP(&opts.AddCollaborators, "add-collaborator", "c", nil, "Add a collaborator with format 'username:permission' (permission: pull, push, admin, maintain, triage)")
	cmd.Flags().StringSliceVar(&opts.RemoveCollaborators, "remove-collaborator", nil, "Remove a collaborator by username")
	cmd.Flags().StringSliceVar(&opts.AddTeams, "add-team", nil, "Add a team with format 'org/team:permission' (permission: pull, push, admin, maintain, triage)")
	cmd.Flags().StringSliceVar(&opts.RemoveTeams, "remove-team", nil, "Remove a team by slug (format: org/team)")

	return cmd
}

func editRun(ctx context.Context, opts *EditOptions) error {
	repo := opts.Repository

	if opts.InteractiveMode {
		detector := opts.Detector
		if detector == nil {
			cachedClient := api.NewCachedHTTPClient(opts.HTTPClient, time.Hour*24)
			detector = fd.NewDetector(cachedClient, repo.RepoHost())
		}
		repoFeatures, err := detector.RepositoryFeatures()
		if err != nil {
			return err
		}

		apiClient := api.NewClientFromHTTP(opts.HTTPClient)
		fieldsToRetrieve := []string{
			"defaultBranchRef",
			"deleteBranchOnMerge",
			"description",
			"hasIssuesEnabled",
			"hasProjectsEnabled",
			"hasWikiEnabled",
			// TODO: GitHub Enterprise Server does not support has_discussions yet
			// "hasDiscussionsEnabled",
			"homepageUrl",
			"isInOrganization",
			"isTemplate",
			"mergeCommitAllowed",
			"rebaseMergeAllowed",
			"repositoryTopics",
			"stargazerCount",
			"squashMergeAllowed",
			"watchers",
		}
		if repoFeatures.VisibilityField {
			fieldsToRetrieve = append(fieldsToRetrieve, "visibility")
		}
		if repoFeatures.AutoMerge {
			fieldsToRetrieve = append(fieldsToRetrieve, "autoMergeAllowed")
		}

		opts.IO.StartProgressIndicator()
		fetchedRepo, err := api.FetchRepository(apiClient, opts.Repository, fieldsToRetrieve)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
		err = interactiveRepoEdit(ctx, opts, fetchedRepo)
		if err != nil {
			return err
		}
	}

	if opts.Edits.SecurityAndAnalysis != nil {
		apiClient := api.NewClientFromHTTP(opts.HTTPClient)
		repo, err := api.FetchRepository(apiClient, opts.Repository, []string{"viewerCanAdminister"})
		if err != nil {
			return err
		}
		if !repo.ViewerCanAdminister {
			return fmt.Errorf("you do not have sufficient permissions to edit repository security and analysis features")
		}
	}

	apiPath := fmt.Sprintf("repos/%s/%s", repo.RepoOwner(), repo.RepoName())

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(opts.Edits); err != nil {
		return err
	}

	g := errgroup.Group{}

	if body.Len() > 3 {
		g.Go(func() error {
			apiClient := api.NewClientFromHTTP(opts.HTTPClient)
			_, err := api.CreateRepoTransformToV4(apiClient, repo.RepoHost(), "PATCH", apiPath, body)
			return err
		})
	}

	if len(opts.AddTopics) > 0 || len(opts.RemoveTopics) > 0 {
		g.Go(func() error {
			// opts.topicsCache gets populated in interactive mode
			if !opts.InteractiveMode {
				var err error
				opts.topicsCache, err = getTopics(ctx, opts.HTTPClient, repo)
				if err != nil {
					return err
				}
			}
			oldTopics := set.NewStringSet()
			oldTopics.AddValues(opts.topicsCache)

			newTopics := set.NewStringSet()
			newTopics.AddValues(opts.topicsCache)
			newTopics.AddValues(opts.AddTopics)
			newTopics.RemoveValues(opts.RemoveTopics)

			if oldTopics.Equal(newTopics) {
				return nil
			}
			return setTopics(ctx, opts.HTTPClient, repo, newTopics.ToSlice())
		})
	}

	// Handle collaborator management
	apiClient := api.NewClientFromHTTP(opts.HTTPClient)
	for _, collabInput := range opts.AddCollaborators {
		collabInput := collabInput // capture loop variable
		g.Go(func() error {
			parts := strings.SplitN(collabInput, ":", 2)
			username := strings.TrimSpace(parts[0])
			permission := strings.ToLower(strings.TrimSpace(parts[1]))
			return addCollaborator(ctx, apiClient, repo, username, permission)
		})
	}

	for _, username := range opts.RemoveCollaborators {
		username := username // capture loop variable
		g.Go(func() error {
			return removeCollaborator(ctx, apiClient, repo, username)
		})
	}

	for _, teamInput := range opts.AddTeams {
		teamInput := teamInput // capture loop variable
		g.Go(func() error {
			parts := strings.SplitN(teamInput, ":", 2)
			teamSlug := strings.TrimSpace(parts[0])
			permission := strings.ToLower(strings.TrimSpace(parts[1]))
			return addTeam(ctx, apiClient, repo, teamSlug, permission)
		})
	}

	for _, teamSlug := range opts.RemoveTeams {
		teamSlug := teamSlug // capture loop variable
		g.Go(func() error {
			return removeTeam(ctx, apiClient, repo, teamSlug)
		})
	}

	err := g.Wait()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out,
			"%s Edited repository %s\n",
			cs.SuccessIcon(),
			ghrepo.FullName(repo))
	}

	return nil
}

func interactiveChoice(p iprompter, r *api.Repository) ([]string, error) {
	options := []string{
		optionDefaultBranchName,
		optionDescription,
		optionHomePageURL,
		optionIssues,
		optionMergeOptions,
		optionProjects,
		// TODO: GitHub Enterprise Server does not support has_discussions yet
		// optionDiscussions,
		optionTemplateRepo,
		optionTopics,
		optionVisibility,
		optionWikis,
		optionCollaborators,
	}
	if r.IsInOrganization {
		options = append(options, optionAllowForking)
	}
	var answers []string
	selected, err := p.MultiSelect("What do you want to edit?", nil, options)
	if err != nil {
		return nil, err
	}
	for _, i := range selected {
		answers = append(answers, options[i])
	}

	return answers, err
}

func interactiveRepoEdit(ctx context.Context, opts *EditOptions, r *api.Repository) error {
	for _, v := range r.RepositoryTopics.Nodes {
		opts.topicsCache = append(opts.topicsCache, v.Topic.Name)
	}
	p := opts.Prompter
	choices, err := interactiveChoice(p, r)
	if err != nil {
		return err
	}
	for _, c := range choices {
		switch c {
		case optionDescription:
			answer, err := p.Input("Description of the repository", r.Description)
			if err != nil {
				return err
			}
			opts.Edits.Description = &answer
		case optionHomePageURL:
			a, err := p.Input("Repository home page URL", r.HomepageURL)
			if err != nil {
				return err
			}
			opts.Edits.Homepage = &a
		case optionTopics:
			addTopics, err := p.Input("Add topics?(csv format)", "")
			if err != nil {
				return err
			}
			if len(strings.TrimSpace(addTopics)) > 0 {
				opts.AddTopics = parseTopics(addTopics)
			}

			if len(opts.topicsCache) > 0 {
				selected, err := p.MultiSelect("Remove Topics", nil, opts.topicsCache)
				if err != nil {
					return err
				}
				for _, i := range selected {
					opts.RemoveTopics = append(opts.RemoveTopics, opts.topicsCache[i])
				}
			}
		case optionDefaultBranchName:
			name, err := p.Input("Default branch name", r.DefaultBranchRef.Name)
			if err != nil {
				return err
			}
			opts.Edits.DefaultBranch = &name
		case optionWikis:
			c, err := p.Confirm("Enable Wikis?", r.HasWikiEnabled)
			if err != nil {
				return err
			}
			opts.Edits.EnableWiki = &c
		case optionIssues:
			a, err := p.Confirm("Enable Issues?", r.HasIssuesEnabled)
			if err != nil {
				return err
			}
			opts.Edits.EnableIssues = &a
		case optionProjects:
			a, err := p.Confirm("Enable Projects?", r.HasProjectsEnabled)
			if err != nil {
				return err
			}
			opts.Edits.EnableProjects = &a
		case optionVisibility:
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Danger zone: changing repository visibility can have unexpected consequences; consult https://gh.io/setting-repository-visibility before continuing.\n", cs.WarningIcon())

			visibilityOptions := []string{"public", "private", "internal"}
			selected, err := p.Select("Visibility", strings.ToLower(r.Visibility), visibilityOptions)
			if err != nil {
				return err
			}
			selectedVisibility := visibilityOptions[selected]

			if selectedVisibility != r.Visibility && (r.StargazerCount > 0 || r.Watchers.TotalCount > 0) {
				fmt.Fprintf(opts.IO.ErrOut, "%s Changing the repository visibility to %s will cause permanent loss of %s and %s.\n", cs.WarningIcon(), selectedVisibility, text.Pluralize(r.StargazerCount, "star"), text.Pluralize(r.Watchers.TotalCount, "watcher"))
			}

			confirmed, err := p.Confirm(fmt.Sprintf("Do you want to change visibility to %s?", selectedVisibility), false)
			if err != nil {
				return err
			}
			if confirmed {
				opts.Edits.Visibility = &selectedVisibility
			}
		case optionMergeOptions:
			var defaultMergeOptions []string
			var selectedMergeOptions []string
			if r.MergeCommitAllowed {
				defaultMergeOptions = append(defaultMergeOptions, allowMergeCommits)
			}
			if r.SquashMergeAllowed {
				defaultMergeOptions = append(defaultMergeOptions, allowSquashMerge)
			}
			if r.RebaseMergeAllowed {
				defaultMergeOptions = append(defaultMergeOptions, allowRebaseMerge)
			}
			mergeOpts := []string{allowMergeCommits, allowSquashMerge, allowRebaseMerge}
			selected, err := p.MultiSelect(
				"Allowed merge strategies",
				defaultMergeOptions,
				mergeOpts)
			if err != nil {
				return err
			}
			for _, i := range selected {
				selectedMergeOptions = append(selectedMergeOptions, mergeOpts[i])
			}
			enableMergeCommit := isIncluded(allowMergeCommits, selectedMergeOptions)
			opts.Edits.EnableMergeCommit = &enableMergeCommit
			enableSquashMerge := isIncluded(allowSquashMerge, selectedMergeOptions)
			opts.Edits.EnableSquashMerge = &enableSquashMerge
			enableRebaseMerge := isIncluded(allowRebaseMerge, selectedMergeOptions)
			opts.Edits.EnableRebaseMerge = &enableRebaseMerge
			if !enableMergeCommit && !enableSquashMerge && !enableRebaseMerge {
				return fmt.Errorf("you need to allow at least one merge strategy")
			}

			opts.Edits.EnableAutoMerge = &r.AutoMergeAllowed
			c, err := p.Confirm("Enable Auto Merge?", r.AutoMergeAllowed)
			if err != nil {
				return err
			}
			opts.Edits.EnableAutoMerge = &c

			opts.Edits.DeleteBranchOnMerge = &r.DeleteBranchOnMerge
			c, err = p.Confirm(
				"Automatically delete head branches after merging?", r.DeleteBranchOnMerge)
			if err != nil {
				return err
			}
			opts.Edits.DeleteBranchOnMerge = &c
		case optionTemplateRepo:
			c, err := p.Confirm("Convert into a template repository?", r.IsTemplate)
			if err != nil {
				return err
			}
			opts.Edits.IsTemplate = &c
		case optionAllowForking:
			c, err := p.Confirm(
				"Allow forking (of an organization repository)?",
				r.ForkingAllowed)
			if err != nil {
				return err
			}
			opts.Edits.AllowForking = &c
		case optionCollaborators:
			err := interactiveCollaboratorManagement(ctx, opts, r)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func parseTopics(s string) []string {
	topics := strings.Split(s, ",")
	for i, topic := range topics {
		topics[i] = strings.TrimSpace(topic)
	}
	return topics
}

func getTopics(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface) ([]string, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/topics", repo.RepoOwner(), repo.RepoName())
	req, err := http.NewRequestWithContext(ctx, "GET", ghinstance.RESTPrefix(repo.RepoHost())+apiPath, nil)
	if err != nil {
		return nil, err
	}

	// "mercy-preview" is still needed for some GitHub Enterprise versions
	req.Header.Set("Accept", "application/vnd.github.mercy-preview+json")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(res)
	}

	var responseData struct {
		Names []string `json:"names"`
	}
	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&responseData)
	return responseData.Names, err
}

func setTopics(ctx context.Context, httpClient *http.Client, repo ghrepo.Interface, topics []string) error {
	payload := struct {
		Names []string `json:"names"`
	}{
		Names: topics,
	}
	body := &bytes.Buffer{}
	dec := json.NewEncoder(body)
	if err := dec.Encode(&payload); err != nil {
		return err
	}

	apiPath := fmt.Sprintf("repos/%s/%s/topics", repo.RepoOwner(), repo.RepoName())
	req, err := http.NewRequestWithContext(ctx, "PUT", ghinstance.RESTPrefix(repo.RepoHost())+apiPath, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-type", "application/json")
	// "mercy-preview" is still needed for some GitHub Enterprise versions
	req.Header.Set("Accept", "application/vnd.github.mercy-preview+json")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return api.HandleHTTPError(res)
	}

	if res.Body != nil {
		_, _ = io.Copy(io.Discard, res.Body)
	}

	return nil
}

func isIncluded(value string, opts []string) bool {
	for _, opt := range opts {
		if strings.EqualFold(opt, value) {
			return true
		}
	}
	return false
}

func boolToStatus(status bool) *string {
	var result string
	if status {
		result = "enabled"
	} else {
		result = "disabled"
	}
	return &result
}

func hasSecurityEdits(edits EditRepositoryInput) bool {
	return edits.enableAdvancedSecurity != nil || edits.enableSecretScanning != nil || edits.enableSecretScanningPushProtection != nil
}

func isValidPermission(permission string) bool {
	validPermissions := []string{"pull", "push", "admin", "maintain", "triage"}
	for _, p := range validPermissions {
		if permission == p {
			return true
		}
	}
	return false
}

func interactiveCollaboratorManagement(ctx context.Context, opts *EditOptions, r *api.Repository) error {
	p := opts.Prompter
	apiClient := api.NewClientFromHTTP(opts.HTTPClient)

	collabOptions := []string{"Add Collaborator", "Remove Collaborator"}
	if r.IsInOrganization {
		collabOptions = append(collabOptions, "Add Team", "Remove Team")
	}

	selected, err := p.Select("What would you like to do?", "", collabOptions)
	if err != nil {
		return err
	}

	switch collabOptions[selected] {
	case "Add Collaborator":
		username, err := p.Input("Username of collaborator to add", "")
		if err != nil {
			return err
		}
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}

		permissionOptions := []string{"pull", "push", "admin", "maintain", "triage"}
		permissionIndex, err := p.Select("Permission level", "push", permissionOptions)
		if err != nil {
			return err
		}
		permission := permissionOptions[permissionIndex]

		opts.AddCollaborators = append(opts.AddCollaborators, fmt.Sprintf("%s:%s", username, permission))

	case "Remove Collaborator":
		collaborators, err := getCollaborators(ctx, apiClient, opts.Repository)
		if err != nil {
			return fmt.Errorf("failed to fetch collaborators: %w", err)
		}

		if len(collaborators) == 0 {
			fmt.Fprintf(opts.IO.ErrOut, "No collaborators found.\n")
			return nil
		}

		collabNames := make([]string, len(collaborators))
		for i, c := range collaborators {
			collabNames[i] = c.Login
		}

		selected, err := p.MultiSelect("Select collaborators to remove", nil, collabNames)
		if err != nil {
			return err
		}

		for _, i := range selected {
			opts.RemoveCollaborators = append(opts.RemoveCollaborators, collabNames[i])
		}

	case "Add Team":
		if !r.IsInOrganization {
			return fmt.Errorf("teams can only be added to organization repositories")
		}

		teamSlug, err := p.Input("Team slug (format: org/team)", "")
		if err != nil {
			return err
		}
		if teamSlug == "" {
			return fmt.Errorf("team slug cannot be empty")
		}

		permissionOptions := []string{"pull", "push", "admin", "maintain", "triage"}
		permissionIndex, err := p.Select("Permission level", "push", permissionOptions)
		if err != nil {
			return err
		}
		permission := permissionOptions[permissionIndex]

		opts.AddTeams = append(opts.AddTeams, fmt.Sprintf("%s:%s", teamSlug, permission))

	case "Remove Team":
		if !r.IsInOrganization {
			return fmt.Errorf("teams can only be removed from organization repositories")
		}

		teams, err := getTeams(ctx, apiClient, opts.Repository)
		if err != nil {
			return fmt.Errorf("failed to fetch teams: %w", err)
		}

		if len(teams) == 0 {
			fmt.Fprintf(opts.IO.ErrOut, "No teams found.\n")
			return nil
		}

		teamNames := make([]string, len(teams))
		for i, t := range teams {
			teamNames[i] = fmt.Sprintf("%s/%s", t.Organization.Login, t.Slug)
		}

		selected, err := p.MultiSelect("Select teams to remove", nil, teamNames)
		if err != nil {
			return err
		}

		for _, i := range selected {
			opts.RemoveTeams = append(opts.RemoveTeams, teamNames[i])
		}
	}

	return nil
}

type Collaborator struct {
	Login       string `json:"login"`
	Permissions struct {
		Admin   bool `json:"admin"`
		Push    bool `json:"push"`
		Pull    bool `json:"pull"`
		Maintain bool `json:"maintain"`
		Triage  bool `json:"triage"`
	} `json:"permissions"`
}

func getCollaborators(ctx context.Context, client *api.Client, repo ghrepo.Interface) ([]Collaborator, error) {
	path := fmt.Sprintf("repos/%s/%s/collaborators",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()))

	var collaborators []Collaborator
	err := client.REST(repo.RepoHost(), "GET", path, nil, &collaborators)
	if err != nil {
		return nil, err
	}

	return collaborators, nil
}

type Team struct {
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
	Permission string `json:"permission"`
}

func getTeams(ctx context.Context, client *api.Client, repo ghrepo.Interface) ([]Team, error) {
	path := fmt.Sprintf("repos/%s/%s/teams",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()))

	var teams []Team
	err := client.REST(repo.RepoHost(), "GET", path, nil, &teams)
	if err != nil {
		return nil, err
	}

	return teams, nil
}

func addCollaborator(ctx context.Context, client *api.Client, repo ghrepo.Interface, username, permission string) error {
	path := fmt.Sprintf("repos/%s/%s/collaborators/%s",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()),
		url.PathEscape(username))

	payload := struct {
		Permission string `json:"permission"`
	}{
		Permission: permission,
	}

	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(payload); err != nil {
		return err
	}

	return client.REST(repo.RepoHost(), "PUT", path, body, nil)
}

func removeCollaborator(ctx context.Context, client *api.Client, repo ghrepo.Interface, username string) error {
	path := fmt.Sprintf("repos/%s/%s/collaborators/%s",
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()),
		url.PathEscape(username))

	return client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}

func addTeam(ctx context.Context, client *api.Client, repo ghrepo.Interface, teamSlug, permission string) error {
	// Parse teamSlug which should be in format "org/team"
	parts := strings.SplitN(teamSlug, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid team format: %s (expected 'org/team')", teamSlug)
	}
	orgSlug := parts[0]
	teamName := parts[1]

	path := fmt.Sprintf("orgs/%s/teams/%s/repos/%s/%s",
		url.PathEscape(orgSlug),
		url.PathEscape(teamName),
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()))

	payload := struct {
		Permission string `json:"permission"`
	}{
		Permission: permission,
	}

	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(payload); err != nil {
		return err
	}

	return client.REST(repo.RepoHost(), "PUT", path, body, nil)
}

func removeTeam(ctx context.Context, client *api.Client, repo ghrepo.Interface, teamSlug string) error {
	// Parse teamSlug which should be in format "org/team"
	parts := strings.SplitN(teamSlug, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid team format: %s (expected 'org/team')", teamSlug)
	}
	orgSlug := parts[0]
	teamName := parts[1]

	path := fmt.Sprintf("orgs/%s/teams/%s/repos/%s/%s",
		url.PathEscape(orgSlug),
		url.PathEscape(teamName),
		url.PathEscape(repo.RepoOwner()),
		url.PathEscape(repo.RepoName()))

	return client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
}

type SecurityAndAnalysisInput struct {
	EnableAdvancedSecurity             *SecurityAndAnalysisStatus `json:"advanced_security,omitempty"`
	EnableSecretScanning               *SecurityAndAnalysisStatus `json:"secret_scanning,omitempty"`
	EnableSecretScanningPushProtection *SecurityAndAnalysisStatus `json:"secret_scanning_push_protection,omitempty"`
}

type SecurityAndAnalysisStatus struct {
	Status *string `json:"status,omitempty"`
}

// Transform security and analysis parameters to properly serialize EditRepositoryInput
// See API Docs: https://docs.github.com/en/rest/repos/repos?apiVersion=2022-11-28#update-a-repository
func transformSecurityAndAnalysisOpts(opts *EditOptions) *SecurityAndAnalysisInput {
	securityOptions := &SecurityAndAnalysisInput{}
	if opts.Edits.enableAdvancedSecurity != nil {
		securityOptions.EnableAdvancedSecurity = &SecurityAndAnalysisStatus{
			Status: boolToStatus(*opts.Edits.enableAdvancedSecurity),
		}
	}
	if opts.Edits.enableSecretScanning != nil {
		securityOptions.EnableSecretScanning = &SecurityAndAnalysisStatus{
			Status: boolToStatus(*opts.Edits.enableSecretScanning),
		}
	}
	if opts.Edits.enableSecretScanningPushProtection != nil {
		securityOptions.EnableSecretScanningPushProtection = &SecurityAndAnalysisStatus{
			Status: boolToStatus(*opts.Edits.enableSecretScanningPushProtection),
		}
	}
	return securityOptions
}
