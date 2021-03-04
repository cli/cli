package delete

import (
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/release/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName     string
	SkipConfirm bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
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

	release, err := shared.FetchRelease(httpClient, baseRepo, opts.TagName)
	if err != nil {
		return err
	}

	if !opts.SkipConfirm && opts.IO.CanPrompt() {
		var confirmed bool
		err := prompt.SurveyAskOne(&survey.Confirm{
			Message: fmt.Sprintf("Delete release %s in %s?", release.TagName, ghrepo.FullName(baseRepo)),
			Default: true,
		}, &confirmed)
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

	if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
		return nil
	}

	iofmt := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted release %s\n", iofmt.SuccessIconWithColor(iofmt.Red), release.TagName)
	if !release.IsDraft {
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
