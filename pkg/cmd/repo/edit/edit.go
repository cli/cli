package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

const (
	allowMergeCommits = "Allow Merge Commits"
	allowSquashMerge  = "Allow Squash Merging"
	allowRebaseMerge  = "Allow Rebase Merging"

	optionAllowForking      = "Allow Forking"
	optionDefaultBranchName = "Default Branch Name"
	optionDescription       = "Description"
	optionHomePageURL       = "Home Page URL"
	optionIssues            = "Issues"
	optionMergeOptions      = "Merge Options"
	optionProjects          = "Projects"
	optionDiscussions       = "Discussions"
	optionTemplateRepo      = "Template Repository"
	optionTopics            = "Topics"
	optionVisibility        = "Visibility"
	optionWikis             = "Wikis"
)

type EditOptions struct {
	HTTPClient      *http.Client
	Repository      ghrepo.Interface
	IO              *iostreams.IOStreams
	Edits           EditRepositoryInput
	AddTopics       []string
	RemoveTopics    []string
	InteractiveMode bool
	Detector        fd.Detector
	Prompter        prompter.Prompter
	// Cache of current repo topics to avoid retrieving them
	// in multiple flows.
	topicsCache []string
}

type EditRepositoryInput struct {
	AllowForking        *bool   `json:"allow_forking,omitempty"`
	AllowUpdateBranch   *bool   `json:"allow_update_branch,omitempty"`
	DefaultBranch       *string `json:"default_branch,omitempty"`
	DeleteBranchOnMerge *bool   `json:"delete_branch_on_merge,omitempty"`
	Description         *string `json:"description,omitempty"`
	EnableAutoMerge     *bool   `json:"allow_auto_merge,omitempty"`
	EnableIssues        *bool   `json:"has_issues,omitempty"`
	EnableMergeCommit   *bool   `json:"allow_merge_commit,omitempty"`
	EnableProjects      *bool   `json:"has_projects,omitempty"`
	EnableDiscussions   *bool   `json:"has_discussions,omitempty"`
	EnableRebaseMerge   *bool   `json:"allow_rebase_merge,omitempty"`
	EnableSquashMerge   *bool   `json:"allow_squash_merge,omitempty"`
	EnableWiki          *bool   `json:"has_wiki,omitempty"`
	Homepage            *string `json:"homepage,omitempty"`
	IsTemplate          *bool   `json:"is_template,omitempty"`
	Visibility          *string `json:"visibility,omitempty"`
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

			To toggle a setting off, use the %[1]s--flag=false%[1]s syntax.

			Note that changing repository visibility to private will cause loss of stars and watchers.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# enable issues and wiki
			gh repo edit --enable-issues --enable-wiki

			# disable projects
			gh repo edit --enable-projects=false
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
	cmdutil.NilBoolFlag(cmd, &opts.Edits.DeleteBranchOnMerge, "delete-branch-on-merge", "", "Delete head branch when pull requests are merged")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.AllowForking, "allow-forking", "", "Allow forking of an organization repository")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.AllowUpdateBranch, "allow-update-branch", "", "Allow a pull request head branch that is behind its base branch to be updated")
	cmd.Flags().StringSliceVar(&opts.AddTopics, "add-topic", nil, "Add repository topic")
	cmd.Flags().StringSliceVar(&opts.RemoveTopics, "remove-topic", nil, "Remove repository topic")

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
		err = interactiveRepoEdit(opts, fetchedRepo)
		if err != nil {
			return err
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

func interactiveChoice(r *api.Repository) ([]string, error) {
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
	}
	if r.IsInOrganization {
		options = append(options, optionAllowForking)
	}
	var answers []string
	//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
	err := prompt.SurveyAskOne(&survey.MultiSelect{
		Message: "What do you want to edit?",
		Options: options,
	}, &answers, survey.WithPageSize(11))
	return answers, err
}

func interactiveRepoEdit(opts *EditOptions, r *api.Repository) error {
	for _, v := range r.RepositoryTopics.Nodes {
		opts.topicsCache = append(opts.topicsCache, v.Topic.Name)
	}
	choices, err := interactiveChoice(r)
	if err != nil {
		return err
	}
	for _, c := range choices {
		switch c {
		case optionDescription:
			opts.Edits.Description = &r.Description
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Input{
				Message: "Description of the repository",
				Default: r.Description,
			}, opts.Edits.Description)
			if err != nil {
				return err
			}
		case optionHomePageURL:
			opts.Edits.Homepage = &r.HomepageURL
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Input{
				Message: "Repository home page URL",
				Default: r.HomepageURL,
			}, opts.Edits.Homepage)
			if err != nil {
				return err
			}
		case optionTopics:
			var addTopics string
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Input{
				Message: "Add topics?(csv format)",
			}, &addTopics)
			if err != nil {
				return err
			}
			if len(strings.TrimSpace(addTopics)) > 0 {
				opts.AddTopics = parseTopics(addTopics)
			}

			if len(opts.topicsCache) > 0 {
				//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
				err = prompt.SurveyAskOne(&survey.MultiSelect{
					Message: "Remove Topics",
					Options: opts.topicsCache,
				}, &opts.RemoveTopics)
				if err != nil {
					return err
				}
			}
		case optionDefaultBranchName:
			opts.Edits.DefaultBranch = &r.DefaultBranchRef.Name
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Input{
				Message: "Default branch name",
				Default: r.DefaultBranchRef.Name,
			}, opts.Edits.DefaultBranch)
			if err != nil {
				return err
			}
		case optionWikis:
			opts.Edits.EnableWiki = &r.HasWikiEnabled
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Enable Wikis?",
				Default: r.HasWikiEnabled,
			}, opts.Edits.EnableWiki)
			if err != nil {
				return err
			}
		case optionIssues:
			opts.Edits.EnableIssues = &r.HasIssuesEnabled
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Enable Issues?",
				Default: r.HasIssuesEnabled,
			}, opts.Edits.EnableIssues)
			if err != nil {
				return err
			}
		case optionProjects:
			opts.Edits.EnableProjects = &r.HasProjectsEnabled
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Enable Projects?",
				Default: r.HasProjectsEnabled,
			}, opts.Edits.EnableProjects)
			if err != nil {
				return err
			}
		case optionDiscussions:
			opts.Edits.EnableDiscussions = &r.HasDiscussionsEnabled
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Enable Discussions?",
				Default: r.HasDiscussionsEnabled,
			}, opts.Edits.EnableDiscussions)
			if err != nil {
				return err
			}
		case optionVisibility:
			opts.Edits.Visibility = &r.Visibility
			visibilityOptions := []string{"public", "private", "internal"}
			selected, err := opts.Prompter.Select("Visibility", strings.ToLower(r.Visibility), visibilityOptions)
			if err != nil {
				return err
			}
			confirmed := true
			if visibilityOptions[selected] == "private" &&
				(r.StargazerCount > 0 || r.Watchers.TotalCount > 0) {
				cs := opts.IO.ColorScheme()
				fmt.Fprintf(opts.IO.ErrOut, "%s Changing the repository visibility to private will cause permanent loss of stars and watchers.\n", cs.WarningIcon())
				confirmed, err = opts.Prompter.Confirm("Do you want to change visibility to private?", false)
				if err != nil {
					return err
				}
			}
			if confirmed {
				opts.Edits.Visibility = &visibilityOptions[selected]
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
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.MultiSelect{
				Message: "Allowed merge strategies",
				Default: defaultMergeOptions,
				Options: []string{allowMergeCommits, allowSquashMerge, allowRebaseMerge},
			}, &selectedMergeOptions)
			if err != nil {
				return err
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
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Enable Auto Merge?",
				Default: r.AutoMergeAllowed,
			}, opts.Edits.EnableAutoMerge)
			if err != nil {
				return err
			}

			opts.Edits.DeleteBranchOnMerge = &r.DeleteBranchOnMerge
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Automatically delete head branches after merging?",
				Default: r.DeleteBranchOnMerge,
			}, opts.Edits.DeleteBranchOnMerge)
			if err != nil {
				return err
			}
		case optionTemplateRepo:
			opts.Edits.IsTemplate = &r.IsTemplate
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Convert into a template repository?",
				Default: r.IsTemplate,
			}, opts.Edits.IsTemplate)
			if err != nil {
				return err
			}
		case optionAllowForking:
			opts.Edits.AllowForking = &r.ForkingAllowed
			//nolint:staticcheck // SA1019: prompt.SurveyAskOne is deprecated: use Prompter
			err = prompt.SurveyAskOne(&survey.Confirm{
				Message: "Allow forking (of an organization repository)?",
				Default: r.ForkingAllowed,
			}, opts.Edits.AllowForking)
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
