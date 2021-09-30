package archive

import (
	"errors"

	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
    "github.com/cli/cli/v2/pkg/iostreams"
    "github.com/cli/cli/v2/internal/ghrepo"
	"github.com/spf13/cobra"
)

// M: do we need any optional args?
type ArchiveOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
    IO      *iostreams.IOStreams

    Repository string
}

// M: what does runF do??
func NewCmdArchive(f *cmdutil.Factory, runF func(*CloneOptions) error) *cobra.Command {
	opts := &ArchiveOptions{
		IO:         f.IOStreams,
		httpClient: f.httpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "archive <repository>",
		Args:  cmdutil.MinimumArgs(1, "cannot archive: repository argument required"),
		Short: "Archive a repository",
		Long: heredoc.Doc(`
            Archive a GitHub repository

            ...
        `),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Repository = args[0]

			if runF != nil {
				return runF(opts)
			}

			return archiveRun(opts)
		},
	}

	cmd.SetFlagErrorFunc(
		func(cmd *cobra.Command, err error) error {
			if err == pflag.ErrHelp {
				return err
			}
			return &cmdutil.FlagError{Err: fmt.Errorf("%w\nSeparate git clone flags with '--'.", err)}
		})

	return cmd
}

// M: do the actual archiving here
func archiveRun(opts *ArchiveOptions) error {
	httpClient, err := opts.HttClient()
	if err != nil {
		return err
	}

	config, err := opts.Config()
	if err != nil {
		return err
	}

	apiClient := api.NewClientForomHTTP(httpClient)

	repoIsURL := strings.Contains(opts.Repository, ":")
	repoIsFullName := !repoIsURL && strings.Contains(opts.Repository, "/")

	// Use the API to archive the repo
    // M: we have to make a new function in /api
	return errors.New("Archive Not implemented")
}
