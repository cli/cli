package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/internal/safeurl"
	attapi "github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DownloadOptions struct {
	HttpClient         func() (*http.Client, error)
	ExternalHttpClient func() (*http.Client, error)
	IO                 *iostreams.IOStreams
	Prompter           prompter.Prompter
	BaseRepo           func() (ghrepo.Interface, error)
	OverwriteExisting  bool
	SkipExisting       bool
	TagName            string
	AssetName          string
	FilePatterns       []string
	Destination        string
	OutputFile         string

	Verify           bool
	AttClient        attapi.Client
	SigstoreVerifier verification.SigstoreVerifier
	ReleaseVerifier  shared.Verifier

	// maximum number of simultaneous downloads
	Concurrency int

	ArchiveType string

	AllowEscapeSequences bool
}

func NewCmdDownload(f *cmdutil.Factory, runF func(*DownloadOptions) error) *cobra.Command {
	opts := &DownloadOptions{
		IO:                 f.IOStreams,
		HttpClient:         f.HttpClient,
		ExternalHttpClient: f.ExternalHttpClient,
		Prompter:           f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "download [<tag> | <asset-url>]",
		Short: "Download release assets",
		Long: heredoc.Docf(`
			Download assets from a GitHub release.

			Without an explicit tag name argument, assets are downloaded from the
			latest release in the project. In this case, %[1]s--pattern%[1]s or %[1]s--archive%[1]s
			is required.

			A release asset URL, such as one copied from a release page, can be given
			instead of a tag name to download that single asset.

			With %[1]s--verify%[1]s, before anything is downloaded, the command checks the
			release and each asset:

			- An immutable release must have a release attestation signed by GitHub
			  that lists each asset's name and digest.
			- Each asset's SLSA build provenance attestation must be signed by GitHub
			  Actions and built from the release's repository. The workflow, source
			  commit, and ref that built it are shown.

			You are then asked to confirm the download; when not running interactively,
			the command fails if any check fails. After downloading, each file is checked
			against its digest. When writing to standard output, the asset is checked
			before any of it is written.

			%[1]s--verify%[1]s cannot be used with %[1]s--archive%[1]s or on GitHub Enterprise Server.
		`, "`"),
		Example: heredoc.Doc(`
			# Download all assets from a specific release
			$ gh release download v1.2.3

			# Download only Debian packages for the latest release
			$ gh release download --pattern '*.deb'

			# Specify multiple file patterns
			$ gh release download -p '*.deb' -p '*.rpm'

			# Download the archive of the source code for a release
			$ gh release download v1.2.3 --archive=zip

			# Check provenance before downloading an asset by its URL
			$ gh release download --verify https://github.com/OWNER/REPO/releases/download/v1.2.3/app.tar.gz
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) == 0 {
				if len(opts.FilePatterns) == 0 && opts.ArchiveType == "" {
					return cmdutil.FlagErrorf("`--pattern` or `--archive` is required when downloading the latest release")
				}
			} else if isURLArg(args[0]) {
				repo, tag, asset, err := parseReleaseAssetURL(args[0])
				if err != nil {
					return cmdutil.FlagErrorWrap(err)
				}
				if cmd.Flags().Changed("repo") {
					return cmdutil.FlagErrorf("`--repo` cannot be used with a release asset URL")
				}
				if len(opts.FilePatterns) > 0 || opts.ArchiveType != "" {
					return cmdutil.FlagErrorf("`--pattern` and `--archive` cannot be used with a release asset URL")
				}
				opts.BaseRepo = func() (ghrepo.Interface, error) { return repo, nil }
				opts.TagName = tag
				opts.AssetName = asset
			} else {
				opts.TagName = args[0]
			}

			if opts.Verify {
				if opts.ArchiveType != "" {
					return cmdutil.FlagErrorf("`--verify` cannot be used with `--archive`: source archives are generated on demand and have no attestations")
				}
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
	cmd.Flags().BoolVar(&opts.AllowEscapeSequences, "allow-escape-sequences", false, "Allow printing terminal escape sequences when writing an asset to standard output")
	cmd.Flags().BoolVar(&opts.Verify, "verify", false, "Check release immutability and verify build provenance before downloading")

	cmdutil.DisableAuthCheck(cmd)

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

	if opts.Verify {
		if err := checkVerifySupported(baseRepo); err != nil {
			return err
		}
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
		var archiveURL string
		if opts.ArchiveType == "tar.gz" {
			archiveURL = release.TarballURL
		} else {
			archiveURL = release.ZipballURL
		}

		if archiveURL == "" {
			errMessage := fmt.Sprintf(
				"release %q with tag %q, does not have a %q archive asset.",
				release.Name, release.TagName, opts.ArchiveType,
			)
			if release.IsDraft {
				errMessage += " Most likely, this is because it is a draft."
			}
			return errors.New(errMessage)
		}

		// create pseudo-Asset with no name and pointing to ZipBallURL or TarBallURL
		toDownload = append(toDownload, shared.ReleaseAsset{APIURL: archiveURL})
		isArchive = true
	} else {
		for _, a := range release.Assets {
			if opts.AssetName != "" && a.Name != opts.AssetName {
				continue
			}
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
		if opts.AssetName != "" {
			return fmt.Errorf("release %s has no asset named %q", release.TagName, opts.AssetName)
		}
		if len(release.Assets) > 0 {
			return errors.New("no assets match the file pattern")
		}
		return errors.New("no assets to download")
	}

	if len(toDownload) > 1 && opts.OutputFile != "" {
		return fmt.Errorf("unable to write more than one asset with `--output`, got %d assets", len(toDownload))
	}

	var verifiedDigests map[string]string
	if opts.Verify {
		opts.IO.StopProgressIndicator()
		verifiedDigests, err = confirmProvenance(opts, httpClient, baseRepo, release, toDownload)
		if err != nil {
			return err
		}
	}

	// An asset written to standard output is external content. It funnels through
	// ContentOut so the sink is auditable; the safety decision (refuse binary bound
	// for a terminal or escape sequences in text, unless --allow-escape-sequences)
	// is made per copy below. Writing to a file keeps the raw bytes.
	opts.IO.SetContentSanitization(false)

	dest := &destinationWriter{
		file:         opts.OutputFile,
		dir:          opts.Destination,
		skipExisting: opts.SkipExisting,
		overwrite:    opts.OverwriteExisting,
		stdout:       opts.IO.ContentOut,
		allowEscapes: opts.AllowEscapeSequences,
		isTTY:        opts.IO.IsStdoutTTY(),
	}
	defer dest.Close()

	targets := make([]downloadTarget, len(toDownload))
	for i, a := range toDownload {
		targets[i] = downloadTarget{
			url:            safeurl.NewImmutableSafeURL(a.APIURL),
			name:           a.Name,
			expectedDigest: verifiedDigests[a.Name],
		}
	}

	if !opts.Verify {
		return downloadAssets(dest, httpClient, targets, opts.Concurrency, isArchive, opts.IO)
	}
	return downloadVerified(dest, httpClient, targets, opts)
}

// downloadVerified downloads assets into a private staging directory and
// checks each one against its expected digest there. Only once every asset
// has passed are they written to the destination, one at a time. A failed
// check therefore leaves the destination untouched, nothing unchecked reaches
// standard output, and concurrent workers never share a destination file.
func downloadVerified(dest *destinationWriter, httpClient *http.Client, targets []downloadTarget, opts *DownloadOptions) error {
	seen := make(map[string]string, len(targets))
	pending := targets[:0:0]
	for _, t := range targets {
		folded := strings.ToLower(t.name)
		if other, ok := seen[folded]; ok {
			return fmt.Errorf("assets %q and %q differ only in case and cannot both be downloaded with `--verify`", other, t.name)
		}
		seen[folded] = t.name

		if err := dest.Check(t.name); errors.Is(err, errSkipped) {
			continue
		} else if err != nil {
			return err
		}
		pending = append(pending, t)
	}
	if len(pending) == 0 {
		return nil
	}
	if err := checkHardLinkedDestinations(dest, pending); err != nil {
		return err
	}

	stagingDir, err := os.MkdirTemp("", "gh-release-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagingDir)
	staging := &destinationWriter{dir: stagingDir}
	defer staging.Close()

	if err := downloadAssets(staging, httpClient, pending, opts.Concurrency, false, opts.IO); err != nil {
		return err
	}

	for _, t := range pending {
		if err := publishStaged(dest, staging, t.name); err != nil && !errors.Is(err, errSkipped) {
			return err
		}
	}
	return nil
}

// checkHardLinkedDestinations rejects existing destination files that are the
// same file on disk, since overwriting one would also overwrite the other with
// a different asset's content.
func checkHardLinkedDestinations(dest *destinationWriter, targets []downloadTarget) error {
	if dest.isStdout() {
		return nil
	}
	type existing struct {
		name string
		info os.FileInfo
	}
	var seen []existing
	for _, t := range targets {
		info, err := os.Lstat(dest.makePath(t.name))
		if err != nil {
			continue
		}
		for _, e := range seen {
			if os.SameFile(e.info, info) {
				return fmt.Errorf("%s and %s are the same file on disk (hard links), so one asset would overwrite the other", dest.makePath(e.name), dest.makePath(t.name))
			}
		}
		seen = append(seen, existing{t.name, info})
	}
	return nil
}

func publishStaged(dest, staging *destinationWriter, name string) error {
	f, err := os.Open(staging.makePath(name))
	if err != nil {
		return err
	}
	defer f.Close()
	return dest.Copy(name, f)
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if isMatch, err := filepath.Match(p, name); err == nil && isMatch {
			return true
		}
	}
	return false
}

type downloadTarget struct {
	url  safeurl.SafeURL
	name string
	// expectedDigest is the hex-encoded sha256 the downloaded content must
	// match; empty means no check.
	expectedDigest string
}

func downloadAssets(dest *destinationWriter, httpClient *http.Client, toDownload []downloadTarget, numWorkers int, isArchive bool, io *iostreams.IOStreams) error {
	if numWorkers == 0 {
		return errors.New("the number of concurrent workers needs to be greater than 0")
	}

	jobs := make(chan downloadTarget, len(toDownload))
	results := make(chan error, len(toDownload))

	if len(toDownload) < numWorkers {
		numWorkers = len(toDownload)
	}

	for w := 1; w <= numWorkers; w++ {
		go func() {
			for a := range jobs {
				io.StartProgressIndicatorWithLabel(fmt.Sprintf("Downloading %s", a.name))
				results <- downloadAsset(dest, httpClient, a.url, a.name, isArchive, a.expectedDigest)
			}
		}()
	}

	for _, a := range toDownload {
		jobs <- a
	}
	close(jobs)

	var downloadError error
	for range toDownload {
		if err := <-results; err != nil && !errors.Is(err, errSkipped) {
			downloadError = err
		}
	}

	io.StopProgressIndicator()

	return downloadError
}

func downloadAsset(dest *destinationWriter, httpClient *http.Client, assetURL safeurl.SafeURL, fileName string, isArchive bool, expectedDigest string) error {
	if err := dest.Check(fileName); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodGet, assetURL.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/octet-stream")
	if isArchive {
		// adding application/json to Accept header due to a bug in the zipball/tarball API endpoint that makes it mandatory
		req.Header.Set("Accept", "application/octet-stream, application/json")

		// override HTTP redirect logic to avoid "legacy" Codeload resources. This is client
		// level rather than request level, so the client is copied before being handed over
		// rather than mutating the one shared with every other caller.
		oldClient := *httpClient
		httpClient = &oldClient
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) == 1 {
				req.URL.Path = removeLegacyFromCodeloadPath(req.URL.Path)
			}
			return nil
		}
	}

	// The asset URL is supplied by the API, so it is absolute and requested as given. DoRequest
	// rather than Request because the CheckRedirect above is a property of the client, and
	// Request delegates to go-gh, which builds a client of its own from the transport alone and
	// so silently drops it. Without it the "legacy" Codeload path is never rewritten and the
	// archive is saved under the wrong name.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).DoRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

	if expectedDigest == "" {
		return dest.Copy(fileName, resp.Body)
	}

	hasher := sha256.New()
	if err := dest.Copy(fileName, io.TeeReader(resp.Body, hasher)); err != nil {
		return err
	}
	if actual := hex.EncodeToString(hasher.Sum(nil)); actual != expectedDigest {
		return fmt.Errorf("downloaded %s does not match its attested digest (expected sha256:%s, got sha256:%s); nothing was saved", fileName, expectedDigest, actual)
	}
	return nil
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
	allowEscapes bool
	isTTY        bool
	root         *safepaths.Root
	rootOnce     sync.Once
	rootErr      error
}

func (w *destinationWriter) ensureRoot() error {
	if w.file != "" {
		return nil
	}
	w.rootOnce.Do(func() {
		w.root, w.rootErr = safepaths.OpenRootDir(w.dir, 0o755)
	})
	return w.rootErr
}

func (w *destinationWriter) Close() error {
	if w.root == nil {
		return nil
	}
	return w.root.Close()
}

func (w *destinationWriter) makePath(name string) string {
	if w.file == "" {
		return filepath.Join(w.dir, name)
	}
	return w.file
}

// isStdout reports whether output goes to standard output. Only an explicit
// "--output -" means that; an asset that happens to be named "-" is a file.
func (w *destinationWriter) isStdout() bool {
	return w.file == "-"
}

// Check returns an error if a file already exists at destination
func (w *destinationWriter) Check(name string) error {
	if name == "" {
		// skip check as file name will only be known after the API request
		return nil
	}
	if w.isStdout() {
		// writing to stdout should always proceed
		return nil
	}
	fp := w.makePath(name)
	if err := w.validateName(name); err != nil {
		return err
	}
	return w.check(fp)
}

func (w *destinationWriter) validateName(name string) error {
	if w.file != "" {
		return nil
	}
	return safepaths.ValidateChild(name)
}

func (w *destinationWriter) check(fp string) error {
	_, err := os.Lstat(fp)
	if err == nil {
		if w.skipExisting {
			return errSkipped
		}
		if !w.overwrite {
			return fmt.Errorf(
				"%s already exists (use `--clobber` to overwrite file or `--skip-existing` to skip file)",
				fp,
			)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Copy writes the data from r into a file specified by name.
func (w *destinationWriter) Copy(name string, r io.Reader) (copyErr error) {
	fp := w.makePath(name)
	if w.isStdout() {
		if w.allowEscapes {
			_, copyErr = io.Copy(w.stdout, r)
			return
		}
		copyErr = iostreams.CopyGuardedContent(w.stdout, r, w.isTTY)
		if binErr, ok := errors.AsType[iostreams.BinaryTerminalError](copyErr); ok {
			copyErr = fmt.Errorf("%w; use `--output` to save it to a file, or pass --allow-escape-sequences to output it anyway", binErr)
		} else if errors.Is(copyErr, iostreams.ErrEscapeSequence) {
			copyErr = errors.New("the asset contains terminal escape sequences; use `--output` to save it to a file, or pass --allow-escape-sequences to output it anyway")
		}
		return
	}
	if copyErr = w.validateName(name); copyErr != nil {
		return
	}
	if copyErr = w.check(fp); copyErr != nil {
		return
	}

	var f *os.File
	if w.file == "" {
		if copyErr = w.ensureRoot(); copyErr != nil {
			return
		}
		f, copyErr = w.root.Create(name, 0o644, 0o755, w.overwrite)
	} else {
		f, copyErr = safepaths.OpenFile(fp, 0o644, 0o755, w.overwrite)
	}
	if copyErr != nil {
		if os.IsExist(copyErr) {
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
