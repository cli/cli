package archive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/shurcooL/githubv4"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"

	// change to local import in go.mod
	"github.com/cli/cli/v2/pkg/cmd/repo/shared"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ArchiveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	RepoArg    string
}

func NewCmdArchive(f *cmdutil.Factory, runF func(*ArchiveOptions) error) *cobra.Command {
	opts := &ArchiveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "archive [<repository>]",
		Short: "Archive a repository",
		Long: `Archive a GitHub repository.

        With no argument, the repository for the current directory is archived.`,

		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return archiveRun(opts)
		},
	}

	return cmd
}

func archiveRun(opts *ArchiveOptions) error {
	cs := opts.IO.ColorScheme()
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	var toArchive ghrepo.Interface
	if opts.RepoArg == "" {
		toArchive, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		archiveURL := opts.RepoArg
		if !strings.Contains(archiveURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}
			archiveURL = currentUser + "/" + archiveURL
		}
		toArchive, err = ghrepo.FromFullName(archiveURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}

	fields := []string{"name", "owner", "isArchived", "id"}
	repo, err := shared.FetchRepository(apiClient, toArchive, fields)
	if err != nil {
		return err
	}

	if repo.IsArchived {
		fmt.Fprintf(opts.IO.ErrOut, "%s Repository %s/%s is already archived\n", cs.WarningIcon(), repo.Owner.Login, repo.Name)
		return nil
	}

	err = api.RepoArchive(apiClient, repo)
	if err != nil {
		return fmt.Errorf("API called failed: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Archived repository %s/%s\n", cs.SuccessIconWithColor(cs.Red), repo.Owner.Login, repo.Name)

	return nil
}

func RepoArchive(client *Client, repo *api.Repository) error {
	var mutation struct {
		ArchiveRepository struct {
			api.Repository struct {
				ID string
			}
		} `graphql:"archiveRepository(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ArchiveRepositoryInput{
			RepositoryID: repo.ID,
		},
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.MutateNamed(context.Background(), "ArchiveRepository", &mutation, variables)
	return err
}
