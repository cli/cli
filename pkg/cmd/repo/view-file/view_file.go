package viewfile

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewFileOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	Path string
	Ref  string
}

func NewCmdViewFile(f *cmdutil.Factory, runF func(*ViewFileOptions) error) *cobra.Command {
	opts := &ViewFileOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "view-file <path>",
		Short: "View a file in a repository",
		Long: heredoc.Docf(`
			Display the contents of a file in a GitHub repository without cloning it.

			By default, the file is fetched from the default branch of the repository
			associated with the current directory.

			Use %[1]s-R%[1]s to specify a different repository and %[1]s--ref%[1]s to
			specify a branch, tag, or commit SHA.
		`, "`"),
		Example: heredoc.Doc(`
			# View a file from the current repository
			$ gh repo view-file README.md

			# View a file from a specific branch
			$ gh repo view-file src/main.go --ref feature-branch

			# View a file from another repository
			$ gh repo view-file go.mod -R cli/cli

			# View a file at a specific commit
			$ gh repo view-file config.yaml --ref abc1234

			# Pipe file contents to another command
			$ gh repo view-file data.json -R owner/repo | jq '.version'
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Path = args[0]

			if repoOverride, _ := c.Flags().GetString("repo"); repoOverride != "" {
				opts.BaseRepo = cmdutil.OverrideBaseRepoFunc(f.BaseRepo, repoOverride)
			}

			if runF != nil {
				return runF(opts)
			}
			return viewFileRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Ref, "ref", "", "The branch, tag, or commit SHA to view the file from")

	return cmd
}

func viewFileRun(opts *ViewFileOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	ref := opts.Ref
	if ref == "" {
		ref = "HEAD"
	}

	expression := ref + ":" + opts.Path
	apiClient := api.NewClientFromHTTP(httpClient)

	var result struct {
		Repository struct {
			Object *struct {
				Typename string `json:"__typename"`
				Text     string `json:"text"`
				ByteSize int    `json:"byteSize"`
				IsBinary bool   `json:"isBinary"`
			}
		}
	}

	query := `query RepositoryFileView($owner: String!, $name: String!, $expression: String!) {
		repository(owner: $owner, name: $name) {
			object(expression: $expression) {
				__typename
				... on Blob {
					text
					byteSize
					isBinary
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"owner":      repo.RepoOwner(),
		"name":       repo.RepoName(),
		"expression": expression,
	}

	err = apiClient.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return err
	}

	obj := result.Repository.Object
	if obj == nil {
		return fmt.Errorf("file not found: %s (ref: %s)", opts.Path, ref)
	}

	if obj.Typename != "Blob" {
		return fmt.Errorf("path is a directory, not a file: %s (use `gh repo ls-files` to list directory contents)", opts.Path)
	}

	if obj.IsBinary {
		return fmt.Errorf("file is binary (%d bytes): %s", obj.ByteSize, opts.Path)
	}

	_, err = fmt.Fprint(opts.IO.Out, obj.Text)
	return err
}
