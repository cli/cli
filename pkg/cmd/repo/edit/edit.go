package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

type EditOptions struct {
	HTTPClient      *http.Client
	Repository      ghrepo.Interface
	IO              *iostreams.IOStreams
	Edits           EditRepositoryInput
	AddTopics       []string
	RemoveTopics    []string
	InteractiveMode bool
}

type EditRepositoryInput struct {
	Description         *string `json:"description,omitempty"`
	Homepage            *string `json:"homepage,omitempty"`
	Visibility          *string `json:"visibility,omitempty"`
	EnableIssues        *bool   `json:"has_issues,omitempty"`
	EnableProjects      *bool   `json:"has_projects,omitempty"`
	EnableWiki          *bool   `json:"has_wiki,omitempty"`
	IsTemplate          *bool   `json:"is_template,omitempty"`
	DefaultBranch       *string `json:"default_branch,omitempty"`
	EnableSquashMerge   *bool   `json:"allow_squash_merge,omitempty"`
	EnableMergeCommit   *bool   `json:"allow_merge_commit,omitempty"`
	EnableRebaseMerge   *bool   `json:"allow_rebase_merge,omitempty"`
	EnableAutoMerge     *bool   `json:"allow_auto_merge,omitempty"`
	DeleteBranchOnMerge *bool   `json:"delete_branch_on_merge,omitempty"`
	AllowForking        *bool   `json:"allow_forking,omitempty"`
}

func NewCmdEdit(f *cmdutil.Factory, runF func(options *EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO: f.IOStreams,
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
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().NFlag() == 0 {
				opts.InteractiveMode = true
			}

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
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableMergeCommit, "enable-merge-commit", "", "Enable merging pull requests via merge commit")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableSquashMerge, "enable-squash-merge", "", "Enable merging pull requests via squashed commit")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableRebaseMerge, "enable-rebase-merge", "", "Enable merging pull requests via rebase")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.EnableAutoMerge, "enable-auto-merge", "", "Enable auto-merge functionality")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.DeleteBranchOnMerge, "delete-branch-on-merge", "", "Delete head branch when pull requests are merged")
	cmdutil.NilBoolFlag(cmd, &opts.Edits.AllowForking, "allow-forking", "", "Allow forking of an organization repository")
	cmd.Flags().StringSliceVar(&opts.AddTopics, "add-topic", nil, "Add repository topic")
	cmd.Flags().StringSliceVar(&opts.RemoveTopics, "remove-topic", nil, "Remove repository topic")

	return cmd
}

func editRun(ctx context.Context, opts *EditOptions) error {
	if opts.InteractiveMode {
		apiClient := api.NewClientFromHTTP(opts.HTTPClient)

		opts.IO.StartProgressIndicator()
		fetchedRepo, err := api.FetchRepository(apiClient, opts.Repository, []string{"description", "homepageUrl", "defaultBranchRef", "isInOrganization"})
		if err != nil {
			return err
		}
		opts.IO.StopProgressIndicator()
		editOpts, err := interactiveRepoEdit(fetchedRepo)
		if err != nil {
			return err
		}
		opts.Edits = *editOpts
	}

	repo := opts.Repository
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
			existingTopics, err := getTopics(ctx, opts.HTTPClient, repo)
			if err != nil {
				return err
			}

			oldTopics := set.NewStringSet()
			oldTopics.AddValues(existingTopics)

			newTopics := set.NewStringSet()
			newTopics.AddValues(existingTopics)
			newTopics.AddValues(opts.AddTopics)
			newTopics.RemoveValues(opts.RemoveTopics)

			if oldTopics.Equal(newTopics) {
				return nil
			}
			return setTopics(ctx, opts.HTTPClient, repo, newTopics.ToSlice())
		})
	}

	return g.Wait()
}

func interactiveRepoEdit(r *api.Repository) (*EditRepositoryInput, error) {
	var qs []*survey.Question

	repoDescriptionQuestion := &survey.Question{
		Name: "repoDescription",
		Prompt: &survey.Input{
			Message: "Description of the repository",
			Default: r.Description,
		},
	}

	qs = append(qs, repoDescriptionQuestion)

	repoHomePageURLQuestion := &survey.Question{
		Name: "repoURL",
		Prompt: &survey.Input{
			Message: "Repository home page URL",
			Default: r.HomepageURL,
		},
	}

	qs = append(qs, repoHomePageURLQuestion)

	defaultBranchNameQuestion := &survey.Question{
		Name: "defaultBranchName",
		Prompt: &survey.Input{
			Message: "Default branch name?",
			Default: r.DefaultBranchRef.Name,
		},
	}

	qs = append(qs, defaultBranchNameQuestion)

	enableWikisQuestion := &survey.Question{
		Name: "enableWikis",
		Prompt: &survey.Confirm{
			Message: "Enable Wikis?",
			Default: true,
		},
	}

	qs = append(qs, enableWikisQuestion)

	enableIssuesQuestion := &survey.Question{
		Name: "enableIssues",
		Prompt: &survey.Confirm{
			Message: "Enable Issues?",
			Default: true,
		},
	}

	qs = append(qs, enableIssuesQuestion)

	enableProjectsQuestion := &survey.Question{
		Name: "enableProjects",
		Prompt: &survey.Confirm{
			Message: "Enable Projects?",
			Default: true,
		},
	}

	qs = append(qs, enableProjectsQuestion)

	repoVisibilityQuestion := &survey.Question{
		Name: "repoVisibility",
		Prompt: &survey.Select{
			Message: "Visibility",
			Options: []string{"Public", "Private", "Internal"},
		},
	}
	qs = append(qs, repoVisibilityQuestion)

	mergeOptionsQuestion := &survey.Question{
		Name: "mergeOptions",
		Prompt: &survey.MultiSelect{
			Message: "Choose a merge option",
			Default: []string{"Allow Merge Commits", "Allow Squash Merging", "Allow Rebase Merging"},
			Options: []string{"Allow Merge Commits", "Allow Squash Merging", "Allow Rebase Merging"},
		},
	}
	qs = append(qs, mergeOptionsQuestion)

	enableAutoMergeQuestion := &survey.Question{
		Name: "enableAutoMerge",
		Prompt: &survey.Confirm{
			Message: "Enable Auto Merge?",
			Default: false,
		},
	}
	qs = append(qs, enableAutoMergeQuestion)

	templateRepoQuestion := &survey.Question{
		Name: "isTemplateRepo",
		Prompt: &survey.Confirm{
			Message: "Convert into a template repository?",
			Default: false,
		},
	}

	qs = append(qs, templateRepoQuestion)

	autoDeleteBranchQuestion := &survey.Question{
		Name: "autoDeleteBranch",
		Prompt: &survey.Confirm{
			Message: "Automatically delete head branches after merging?",
			Default: false,
		},
	}

	qs = append(qs, autoDeleteBranchQuestion)

	if r.IsInOrganization {
		allowForkingQuestion := &survey.Question{
			Name: "allowForking",
			Prompt: &survey.Confirm{
				Message: "Allow forking (of an organization repository)?",
				Default: false,
			},
		}

		qs = append(qs, allowForkingQuestion)
	}

	answers := struct {
		RepoDescription   string
		RepoURL           string
		RepoVisibility    string
		MergeOptions      []int
		DefaultBranchName string
		EnableWikis       bool
		EnableIssues      bool
		EnableProjects    bool
		EnableAutoMerge   bool
		IsTemplateRepo    bool
		AutoDeleteBranch  bool
		AllowForking      bool
	}{}

	err := prompt.SurveyAsk(qs, &answers)
	if err != nil {
		return nil, err
	}

	repoInput := &EditRepositoryInput{
		Description:         &answers.RepoDescription,
		Homepage:            &answers.RepoURL,
		Visibility:          &answers.RepoVisibility,
		EnableIssues:        &answers.EnableIssues,
		EnableProjects:      &answers.EnableProjects,
		EnableWiki:          &answers.EnableWikis,
		IsTemplate:          &answers.IsTemplateRepo,
		DefaultBranch:       &answers.DefaultBranchName,
		EnableAutoMerge:     &answers.EnableAutoMerge,
		DeleteBranchOnMerge: &answers.AutoDeleteBranch,
	}

	if r.IsInOrganization {
		repoInput.AllowForking = &answers.AllowForking
	}

	mergeOptions := map[string]bool{
		"0": false,
		"1": false,
		"2": false,
	}

	for _, v := range answers.MergeOptions {
		index := strconv.Itoa(v)
		mergeOptions[index] = true
	}

	if emc, ok := mergeOptions["0"]; ok {
		repoInput.EnableMergeCommit = &emc
	}

	if esm, ok := mergeOptions["1"]; ok {
		repoInput.EnableSquashMerge = &esm
	}

	if erm, ok := mergeOptions["2"]; ok {
		repoInput.EnableRebaseMerge = &erm
	}

	return repoInput, nil
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
		_, _ = io.Copy(ioutil.Discard, res.Body)
	}

	return nil
}
