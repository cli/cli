package deleteasset

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type iprompter interface {
	Confirm(string, bool) (bool, error)
}

type DeleteAssetOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	TagName     string
	SkipConfirm bool
	AssetName   string
}

func NewCmdDeleteAsset(f *cmdutil.Factory, runF func(*DeleteAssetOptions) error) *cobra.Command {
	opts := &DeleteAssetOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete-asset <tag> <asset-name>",
		Short: "Delete an asset from a release",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.TagName = args[0]
			opts.AssetName = args[1]
			if runF != nil {
				return runF(opts)
			}
			return deleteAssetRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.SkipConfirm, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func deleteAssetRun(opts *DeleteAssetOptions) error {
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
			fmt.Sprintf("Delete asset %s in release %s in %s?", opts.AssetName, release.TagName, ghrepo.FullName(baseRepo)),
			true)
		if err != nil {
			return err
		}

		if !confirmed {
			return cmdutil.CancelError
		}
	}

	var assetURL string
	for _, a := range release.Assets {
		if a.Name == opts.AssetName {
			assetURL = a.APIURL
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("asset %s not found in release %s", opts.AssetName, release.TagName)
	}

	err = deleteAsset(httpClient, assetURL)
	if err != nil {
		return err
	}

	if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
		return nil
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted asset %s from release %s\n", cs.SuccessIconWithColor(cs.Red), opts.AssetName, release.TagName)

	return nil
}

func deleteAsset(httpClient *http.Client, assetURL string) error {
	req, err := http.NewRequest("DELETE", assetURL, nil)
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
