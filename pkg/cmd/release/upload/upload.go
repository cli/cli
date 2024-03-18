package upload

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
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
		Long: heredoc.Docf(`
			Upload asset files to a GitHub Release.

			To define a display label for an asset, append text starting with %[1]s#%[1]s after the
			file name.
		`, "`"),
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
		sanitizedFileName := sanitizeFileName(a.Name)
		for _, ea := range release.Assets {
			if ea.Name == sanitizedFileName {
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

// this method attempts to mimic the same functionality on the client that the platform does on
// uploaded assets in order to allow the --clobber logic work correctly, since that feature is
// one that only exists in the client
func sanitizeFileName(name string) string {
	value := text.RemoveDiacritics(name)
	// Stripped all non-ascii characters, provide default name.
	if strings.HasPrefix(value, ".") {
		value = "default" + value
	}

	// Replace special characters with the separator
	value = regexp.MustCompile(`(?i)[^a-z0-9\-_\+@]+`).ReplaceAllLiteralString(value, ".")

	// No more than one of the separator in a row.
	value = regexp.MustCompile(`\.{2,}`).ReplaceAllLiteralString(value, ".")

	// Remove leading/trailing separator.
	value = strings.Trim(value, ".")

	// Just file extension left, add default name.
	if name != value && !strings.Contains(value, ".") {
		value = "default." + value
	}

	return value
}
