package download

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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

	ArchiveType string
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

			# download the archive of the source code for a release
			$ gh release download v1.2.3 --archive=zip
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) == 0 {
				if len(opts.FilePatterns) == 0 && opts.ArchiveType == "" {
					return cmdutil.FlagErrorf("`--pattern` or `--archive` is required when downloading the latest release")
				}
			} else {
				opts.TagName = args[0]
			}

			// check archive type option validity
			if err := checkArchiveTypeOption(opts); err != nil {
				return err
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
	cmd.Flags().StringVarP(&opts.ArchiveType, "archive", "A", "", "Download the source code archive in the specified `format` (zip or tar.gz)")

	return cmd
}

func checkArchiveTypeOption(opts *DownloadOptions) error {
	if len(opts.ArchiveType) == 0 {
		return nil
	}

	if err := cmdutil.MutuallyExclusive(
		"specify only one of '--pattern' or '--archive'",
		true, // ArchiveType len > 0
		len(opts.FilePatterns) > 0,
	); err != nil {
		return err
	}

	if opts.ArchiveType != "zip" && opts.ArchiveType != "tar.gz" {
		return cmdutil.FlagErrorf("the value for `--archive` must be one of \"zip\" or \"tar.gz\"")
	}
	return nil
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

	opts.IO.StartProgressIndicator()
	defer opts.IO.StopProgressIndicator()

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
	isArchive := false
	if opts.ArchiveType != "" {
		var archiveURL = release.ZipballURL
		if opts.ArchiveType == "tar.gz" {
			archiveURL = release.TarballURL
		}
		// create pseudo-Asset with no name and pointing to ZipBallURL or TarBallURL
		toDownload = append(toDownload, shared.ReleaseAsset{APIURL: archiveURL})
		isArchive = true
	} else {
		for _, a := range release.Assets {
			if len(opts.FilePatterns) > 0 && !matchAny(opts.FilePatterns, a.Name) {
				continue
			}
			toDownload = append(toDownload, a)
		}
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

	return downloadAssets(httpClient, toDownload, opts.Destination, opts.Concurrency, isArchive)
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if isMatch, err := filepath.Match(p, name); err == nil && isMatch {
			return true
		}
	}
	return false
}

func downloadAssets(httpClient *http.Client, toDownload []shared.ReleaseAsset, destDir string, numWorkers int, isArchive bool) error {
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
				results <- downloadAsset(httpClient, a.APIURL, destDir, a.Name, isArchive)
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

func downloadAsset(httpClient *http.Client, assetURL, destinationDir string, fileName string, isArchive bool) error {
	req, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/octet-stream")
	if isArchive {
		// adding application/json to Accept header due to a bug in the zipball/tarball API endpoint that makes it mandatory
		req.Header.Set("Accept", "application/octet-stream, application/json")

		// override HTTP redirect logic to avoid "legacy" Codeload resources
		oldClient := *httpClient
		httpClient = &oldClient
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) == 1 {
				req.URL.Path = removeLegacyFromCodeloadPath(req.URL.Path)
			}
			return nil
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	var destinationPath = filepath.Join(destinationDir, fileName)

	if len(fileName) == 0 {
		contentDisposition := resp.Header.Get("Content-Disposition")

		_, params, err := mime.ParseMediaType(contentDisposition)
		if err != nil {
			return fmt.Errorf("unable to parse file name of archive: %w", err)
		}
		if serverFileName, ok := params["filename"]; ok {
			destinationPath = filepath.Join(destinationDir, serverFileName)
		} else {
			return errors.New("unable to determine file name of archive")
		}
	}

	f, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

var codeloadLegacyRE = regexp.MustCompile(`^(/[^/]+/[^/]+/)legacy\.`)

// removeLegacyFromCodeloadPath converts URLs for "legacy" Codeload archives into ones that match the format
// when you choose to download "Source code (zip/tar.gz)" from a tagged release on the web. The legacy URLs
// look like this:
//
//   https://codeload.github.com/OWNER/REPO/legacy.zip/refs/tags/TAGNAME
//
// Removing the "legacy." part results in a valid Codeload URL for our desired archive format.
func removeLegacyFromCodeloadPath(p string) string {
	return codeloadLegacyRE.ReplaceAllString(p, "$1")
}
