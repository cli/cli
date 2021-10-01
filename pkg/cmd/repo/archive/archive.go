package archive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/cmd/repo/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ArchiveOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
    BaseRepo   func() (ghrepo.Interface, error)
    RepoArg    string
}

func NewCmdArchive(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &CloneOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "archive [<repository>]",
		Short: "Archive a repository",
        Long: `Archive a GitHub repository.

        With no argument, the repository for the current directory is archived.`,

		Args:  cmdutil.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) > 0 {
                opts.RepoArg = args[0]
            }

			if runF != nil {
				return runF(&opts)
			}
			return archiveRun(&opts)
		},
	}

	return cmd
}

func archiveRun(opts *ArchiveOptions) error{
    httpClient, err := opts.HttpClient()
    if err != nil {
        return err
    }
    apiClient := api.NewClientFromHTTP(httpClient)

    var toArchive ghrepo.Interface

	if opts.RepoArg == "" {
        toArchive, err := opts.BaseRepo()
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

    fields := []string{ "name", "owner", "isArchived", "id" }
    repo, err := shared.FetchRepository(apiClient, toArchive, fields)

    err = api.RepoArchive(apiClient,repo)
    if err != nil {
        return err
    }


    return nil
}
