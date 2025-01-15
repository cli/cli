package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DownloadOptions struct {
	HttpClient        func() (*http.Client, error)
	IO                *iostreams.IOStreams
	BaseRepo          func() (ghrepo.Interface, error)
	OverwriteExisting bool
	SkipExisting      bool
	TagName           string
	FilePatterns      []string
	Destination       string
	OutputFile        string

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
		Long: heredoc.Docf(`
			Download assets from a GitHub release.

			Without an explicit tag name argument, assets are downloaded from the
			latest release in the project. In this case, %[1]s--pattern%[1]s or %[1]s--archive%[1]s
			is required.
		`, "`"),
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

			if err := cmdutil.MutuallyExclusive("specify only one of `--clobber` or `--skip-existing`", opts.OverwriteExisting, opts.SkipExisting); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--dir` or `--output`", opts.Destination != ".", opts.OutputFile != ""); err != nil {
				return err
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

	cmd.Flags().StringVarP(&opts.OutputFile, "output", "O", "", "The `file` to write a single asset to (use \"-\" to write to standard output)")
	cmd.Flags().StringVarP(&opts.Destination, "dir", "D", ".", "The `directory` to download files into")
	cmd.Flags().StringArrayVarP(&opts.FilePatterns, "pattern", "p", nil, "Download only assets that match a glob pattern")
	cmd.Flags().StringVarP(&opts.ArchiveType, "archive", "A", "", "Download the source code archive in the specified `format` (zip or tar.gz)")
	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing files of the same name")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip downloading when files of the same name exist")

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

	opts.IO.StartProgressIndicatorWithLabel("Finding assets to download")
	defer opts.IO.StopProgressIndicator()

	ctx := context.Background()

	var release *shared.Release
	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(ctx, httpClient, baseRepo)
		if err != nil {
			return err
		}
	} else {
		release, err = shared.FetchRelease(ctx, httpClient, baseRepo, opts.TagName)
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
			// Note that if we need to start checking for reserved filenames on
			// more operating systems we should move to using a build constraints
			// pattern rather than checking the operating system at runtime.
			if runtime.GOOS == "windows" && isWindowsReservedFilename(a.Name) {
				return fmt.Errorf("unable to download release due to asset with reserved filename %q", a.Name)
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

	if len(toDownload) > 1 && opts.OutputFile != "" {
		return fmt.Errorf("unable to write more than one asset with `--output`, got %d assets", len(toDownload))
	}

	dest := destinationWriter{
		file:         opts.OutputFile,
		dir:          opts.Destination,
		skipExisting: opts.SkipExisting,
		overwrite:    opts.OverwriteExisting,
		stdout:       opts.IO.Out,
	}

	return downloadAssets(&dest, httpClient, toDownload, opts.Concurrency, isArchive, opts.IO)
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if isMatch, err := filepath.Match(p, name); err == nil && isMatch {
			return true
		}
	}
	return false
}

func downloadAssets(dest *destinationWriter, httpClient *http.Client, toDownload []shared.ReleaseAsset, numWorkers int, isArchive bool, io *iostreams.IOStreams) error {
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
				io.StartProgressIndicatorWithLabel(fmt.Sprintf("Downloading %s", a.Name))
				results <- downloadAsset(dest, httpClient, a.APIURL, a.Name, isArchive)
			}
		}()
	}

	for _, a := range toDownload {
		jobs <- a
	}
	close(jobs)

	var downloadError error
	for i := 0; i < len(toDownload); i++ {
		if err := <-results; err != nil && !errors.Is(err, errSkipped) {
			downloadError = err
		}
	}

	io.StopProgressIndicator()

	return downloadError
}

func downloadAsset(dest *destinationWriter, httpClient *http.Client, assetURL, fileName string, isArchive bool) error {
	if err := dest.Check(fileName); err != nil {
		return err
	}

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

	if len(fileName) == 0 {
		contentDisposition := resp.Header.Get("Content-Disposition")

		_, params, err := mime.ParseMediaType(contentDisposition)
		if err != nil {
			return fmt.Errorf("unable to parse file name of archive: %w", err)
		}
		if serverFileName, ok := params["filename"]; ok {
			fileName = filepath.Base(serverFileName)
		} else {
			return errors.New("unable to determine file name of archive")
		}
	}

	return dest.Copy(fileName, resp.Body)
}

var codeloadLegacyRE = regexp.MustCompile(`^(/[^/]+/[^/]+/)legacy\.`)

// removeLegacyFromCodeloadPath converts URLs for "legacy" Codeload archives into ones that match the format
// when you choose to download "Source code (zip/tar.gz)" from a tagged release on the web. The legacy URLs
// look like this:
//
//	https://codeload.github.com/OWNER/REPO/legacy.zip/refs/tags/TAGNAME
//
// Removing the "legacy." part results in a valid Codeload URL for our desired archive format.
func removeLegacyFromCodeloadPath(p string) string {
	return codeloadLegacyRE.ReplaceAllString(p, "$1")
}

var errSkipped = errors.New("skipped")

// destinationWriter handles writing content into destination files
type destinationWriter struct {
	file         string
	dir          string
	skipExisting bool
	overwrite    bool
	stdout       io.Writer
}

func (w destinationWriter) makePath(name string) string {
	if w.file == "" {
		return filepath.Join(w.dir, name)
	}
	return w.file
}

// Check returns an error if a file already exists at destination
func (w destinationWriter) Check(name string) error {
	if name == "" {
		// skip check as file name will only be known after the API request
		return nil
	}
	fp := w.makePath(name)
	if fp == "-" {
		// writing to stdout should always proceed
		return nil
	}
	return w.check(fp)
}

func (w destinationWriter) check(fp string) error {
	if _, err := os.Stat(fp); err == nil {
		if w.skipExisting {
			return errSkipped
		}
		if !w.overwrite {
			return fmt.Errorf(
				"%s already exists (use `--clobber` to overwrite file or `--skip-existing` to skip file)",
				fp,
			)
		}
	}
	return nil
}

// Copy writes the data from r into a file specified by name.
func (w destinationWriter) Copy(name string, r io.Reader) (copyErr error) {
	fp := w.makePath(name)
	if fp == "-" {
		_, copyErr = io.Copy(w.stdout, r)
		return
	}
	if copyErr = w.check(fp); copyErr != nil {
		return
	}

	if dir := filepath.Dir(fp); dir != "." {
		if copyErr = os.MkdirAll(dir, 0755); copyErr != nil {
			return
		}
	}

	var f *os.File
	if f, copyErr = os.OpenFile(fp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); copyErr != nil {
		return
	}

	defer func() {
		if err := f.Close(); copyErr == nil && err != nil {
			copyErr = err
		}
	}()

	_, copyErr = io.Copy(f, r)
	return
}

func isWindowsReservedFilename(filename string) bool {
	// Windows terminals should prevent the creation of these files
	// but that behavior is not enforced across terminals. Prevent
	// the user from downloading files with these reserved names as
	// they represent an exploit vector for bad actors.
	// Reserved filenames defined at:
	// https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file#win32-file-namespaces
	reservedFilenames := []string{"CON", "PRN", "AUX", "NUL", "COM0",
		"COM1", "COM2", "COM3", "COM4", "COM5",
		"COM6", "COM7", "COM8", "COM9", "COM¹",
		"COM²", "COM³", "LPT0", "LPT1", "LPT2",
		"LPT3", "LPT4", "LPT5", "LPT6", "LPT7",
		"LPT8", "LPT9", "LPT¹", "LPT²", "LPT³"}

	// Normalize type case and remove file type extension from filename.
	filename = strings.ToUpper(strings.Split(filename, ".")[0])

	return slices.Contains(reservedFilenames, filename)
}
