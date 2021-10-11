package archive

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ArchiveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	RepoArg    string
}

func NewCmdArchive(f *cmdutil.Factory, runF func(*ArchiveOptions) error) *cobra.Command {
	opts := &ArchiveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "archive <repository>",
		Short: "Archive a repository",
		Long:  "Archive a GitHub repository.",
		Args:  cmdutil.ExactArgs(1, "cannot archive: repository argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RepoArg = args[0]
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

	fields := []string{"name", "owner", "isArchived", "id"}
	repo, err := api.FetchRepository(apiClient, toArchive, fields)
	if err != nil {
		return err
	}

	fullName := ghrepo.FullName(toArchive)
	if repo.IsArchived {
		fmt.Fprintf(opts.IO.ErrOut,
			"%s Repository %s is already archived\n",
			cs.WarningIcon(),
			fullName)
		return nil
	}

	err = archiveRepo(httpClient, repo)
	if err != nil {
		return fmt.Errorf("API called failed: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out,
			"%s Archived repository %s\n",
			cs.SuccessIcon(),
			fullName)
	}

	return nil
}
