package download

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/release/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DownloadOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	TagName      string
	FilePatterns []string
	Destination  string

	// maximum number of simultaneous downloads
	Concurrency int
}

func NewCmdDownload(f *cmdutil.Factory, runF func(*DownloadOptions) error) *cobra.Command {
	opts := &DownloadOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "download [<tag>]",
		Short: "Download release assets",
		Long: heredoc.Doc(`
			Download assets from a GitHub release.

			Without an explicit tag name argument, assets are downloaded from the
			latest release in the project. In this case, '--pattern' is required.
		`),
		Example: heredoc.Doc(`
			# download all assets from a specific release
			$ gh release download v1.2.3
			
			# download only Debian packages for the latest release
			$ gh release download --pattern '*.deb'
			
			# specify multiple file patterns
			$ gh release download -p '*.deb' -p '*.rpm'
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) == 0 {
				if len(opts.FilePatterns) == 0 {
					return &cmdutil.FlagError{Err: errors.New("the '--pattern' flag is required when downloading the latest release")}
				}
			} else {
				opts.TagName = args[0]
			}

			opts.Concurrency = 5

			if runF != nil {
				return runF(opts)
			}
			return downloadRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Destination, "dir", "D", ".", "The directory to download files into")
	cmd.Flags().StringArrayVarP(&opts.FilePatterns, "pattern", "p", nil, "Download only assets that match a glob pattern")

	return cmd
}

func downloadRun(opts *DownloadOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	var release *shared.Release

	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(httpClient, baseRepo)
		if err != nil {
			return err
		}
	} else {
		release, err = shared.FetchRelease(httpClient, baseRepo, opts.TagName)
		if err != nil {
			return err
		}
	}

	var toDownload []shared.ReleaseAsset
	for _, a := range release.Assets {
		if len(opts.FilePatterns) > 0 && !matchAny(opts.FilePatterns, a.Name) {
			continue
		}
		toDownload = append(toDownload, a)
	}

	if len(toDownload) == 0 {
		if len(release.Assets) > 0 {
			return errors.New("no assets match the file pattern")
		}
		return errors.New("no assets to download")
	}

	if opts.Destination != "." {
		err := os.MkdirAll(opts.Destination, 0755)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	err = downloadAssets(httpClient, toDownload, opts.Destination, opts.Concurrency)
	opts.IO.StopProgressIndicator()
	return err
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if isMatch, err := filepath.Match(p, name); err == nil && isMatch {
			return true
		}
	}
	return false
}

func downloadAssets(httpClient *http.Client, toDownload []shared.ReleaseAsset, destDir string, numWorkers int) error {
	if numWorkers == 0 {
		return errors.New("the number of concurrent workers needs to be greater than 0")
	}

	jobs := make(chan shared.ReleaseAsset, len(toDownload))
	results := make(chan error, len(toDownload))

	if len(toDownload) < numWorkers {
		numWorkers = len(toDownload)
	}

	for w := 1; w <= numWorkers; w++ {
		go func() {
			for a := range jobs {
				results <- downloadAsset(httpClient, a.APIURL, filepath.Join(destDir, a.Name))
			}
		}()
	}

	for _, a := range toDownload {
		jobs <- a
	}
	close(jobs)

	var downloadError error
	for i := 0; i < len(toDownload); i++ {
		if err := <-results; err != nil {
			downloadError = err
		}
	}

	return downloadError
}

func downloadAsset(httpClient *http.Client, assetURL, destinationPath string) error {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/octet-stream")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	f, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
