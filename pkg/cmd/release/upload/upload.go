package upload

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type UploadOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName string
	Assets  []*shared.AssetForUpload

	// maximum number of simultaneous uploads
	Concurrency       int
	OverwriteExisting bool
}

func NewCmdUpload(f *cmdutil.Factory, runF func(*UploadOptions) error) *cobra.Command {
	opts := &UploadOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "upload <tag> <files>...",
		Short: "Upload assets to a release",
		Long: heredoc.Doc(`
			Upload asset files to a GitHub Release.

			To define a display label for an asset, append text starting with '#' after the
			file name.
		`),
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.TagName = args[0]

			var err error
			opts.Assets, err = shared.AssetsFromArgs(args[1:])
			if err != nil {
				return err
			}

			opts.Concurrency = 5

			if runF != nil {
				return runF(opts)
			}
			return uploadRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing assets of the same name")

	return cmd
}

func uploadRun(opts *UploadOptions) error {
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

	uploadURL := release.UploadURL
	if idx := strings.IndexRune(uploadURL, '{'); idx > 0 {
		uploadURL = uploadURL[:idx]
	}

	var existingNames []string
	for _, a := range opts.Assets {
		for _, ea := range release.Assets {
			if ea.Name == a.Name {
				a.ExistingURL = ea.APIURL
				existingNames = append(existingNames, ea.Name)
				break
			}
		}
	}

	if len(existingNames) > 0 && !opts.OverwriteExisting {
		return fmt.Errorf("asset under the same name already exists: %v", existingNames)
	}

	opts.IO.StartProgressIndicator()
	err = shared.ConcurrentUpload(httpClient, uploadURL, opts.Concurrency, opts.Assets)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		iofmt := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "Successfully uploaded %s to %s\n",
			text.Pluralize(len(opts.Assets), "asset"),
			iofmt.Bold(release.TagName))
	}

	return nil
}
