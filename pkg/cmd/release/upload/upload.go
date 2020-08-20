package upload

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type UploadOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName string
	Assets  []*AssetForUpload

	// maximum number of simultaneous uploads
	Concurrency       int
	OverwriteExisting bool
}

type AssetForUpload struct {
	Name  string
	Label string

	Data     io.ReadCloser
	Size     int64
	MIMEType string

	ExistingURL string
}

func NewCmdUpload(f *cmdutil.Factory, runF func(*UploadOptions) error) *cobra.Command {
	opts := &UploadOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "upload <tag> <files>...",
		Short: "Upload assets to a release",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.TagName = args[0]

			var err error
			opts.Assets, err = assetsFromArgs(args[1:])
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

func assetsFromArgs(args []string) (assets []*AssetForUpload, err error) {
	for _, fn := range args {
		var f *os.File
		var fi os.FileInfo

		f, err = os.Open(fn)
		if err != nil {
			return
		}
		fi, err = f.Stat()
		if err != nil {
			return
		}

		assets = append(assets, &AssetForUpload{
			Data:  f,
			Size:  fi.Size(),
			Name:  filepath.Base(fn),
			Label: "",
			// TODO: infer content type from file extension
			MIMEType: "application/octet-stream",
		})
	}
	return
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

	release, err := fetchRelease(httpClient, baseRepo, opts.TagName)
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
				a.ExistingURL = ea.URL
				existingNames = append(existingNames, ea.Name)
				break
			}
		}
	}

	if len(existingNames) > 0 && !opts.OverwriteExisting {
		return fmt.Errorf("asset under the same name already exists: %v", existingNames)
	}

	opts.IO.StartProgressIndicator()
	err = concurrentUpload(httpClient, uploadURL, opts.Concurrency, opts.Assets)
	opts.IO.EndProgressIndicator()
	if err != nil {
		return err
	}

	return nil
}

func concurrentUpload(httpClient *http.Client, uploadURL string, numWorkers int, assets []*AssetForUpload) error {
	jobs := make(chan AssetForUpload, len(assets))
	results := make(chan error, len(assets))

	for w := 1; w <= numWorkers; w++ {
		go func() {
			for a := range jobs {
				results <- uploadWithDelete(httpClient, uploadURL, a)
			}
		}()
	}

	for _, a := range assets {
		jobs <- *a
	}
	close(jobs)

	var uploadError error
	for i := 0; i < len(assets); i++ {
		if err := <-results; err != nil {
			uploadError = err
		}
	}
	return uploadError
}

func uploadWithDelete(httpClient *http.Client, uploadURL string, a AssetForUpload) error {
	if a.ExistingURL != "" {
		err := deleteAsset(httpClient, a.ExistingURL)
		if err != nil {
			return err
		}
	}

	defer a.Data.Close()
	_, err := uploadAsset(httpClient, uploadURL, a)
	return err
}
