package delete

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Confirm(string, bool) (bool, error)
}

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	TagName     string
	SkipConfirm bool
	CleanupTag  bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete <tag>",
		Short: "Delete a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.TagName = args[0]

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.SkipConfirm, "yes", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&opts.CleanupTag, "cleanup-tag", false, "Delete the specified tag in addition to its release")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	release, err := shared.FetchRelease(context.Background(), httpClient, baseRepo, opts.TagName)
	if err != nil {
		return err
	}

	if !opts.SkipConfirm && opts.IO.CanPrompt() {
		confirmed, err := opts.Prompter.Confirm(
			fmt.Sprintf("Delete release %s in %s?", release.TagName, ghrepo.FullName(baseRepo)), true)
		if err != nil {
			return err
		}

		if !confirmed {
			return cmdutil.CancelError
		}
	}

	err = deleteRelease(httpClient, release.APIURL)
	if err != nil {
		return err
	}

	var cleanupMessage string
	mustCleanupTag := opts.CleanupTag
	if mustCleanupTag {
		err = deleteTag(httpClient, baseRepo, release.TagName)
		if err != nil {
			return err
		}
		cleanupMessage = " and tag"
	}

	if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
		return nil
	}

	iofmt := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted release%s %s\n", iofmt.SuccessIconWithColor(iofmt.Red), cleanupMessage, release.TagName)
	if !release.IsDraft && !mustCleanupTag {
		fmt.Fprintf(opts.IO.ErrOut, "%s Note that the %s git tag still remains in the repository\n", iofmt.WarningIcon(), release.TagName)
	}

	return nil
}

func deleteRelease(httpClient *http.Client, releaseURL string) error {
	req, err := http.NewRequest("DELETE", releaseURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}
	return nil
}

func deleteTag(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) error {
	path := fmt.Sprintf("repos/%s/%s/git/refs/tags/%s", baseRepo.RepoOwner(), baseRepo.RepoName(), tagName)
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}
	return nil
}
