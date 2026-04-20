package copilot

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ci"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safepaths"
	ghzip "github.com/cli/cli/v2/internal/zip"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CopilotOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Prompter   prompter.Prompter

	CopilotArgs []string
	Remove      bool
}

func NewCmdCopilot(f *cmdutil.Factory, runF func(*CopilotOptions) error) *cobra.Command {
	opts := &CopilotOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "copilot [flags] [args]",
		Short: "Run the GitHub Copilot CLI (preview)",
		Long: heredoc.Docf(`
			Runs the GitHub Copilot CLI.

			Executing the Copilot CLI through %[1]sgh%[1]s is currently in preview and subject to change.

			If already installed, %[1]sgh%[1]s will execute the Copilot CLI found in your %[1]sPATH%[1]s.
			If the Copilot CLI is not installed, it will be downloaded to %[2]s.

			Use %[1]s--remove%[1]s to remove the downloaded Copilot CLI.

			This command is only supported on Windows, Linux, and Darwin, on amd64/x64
			or arm64 architectures.

			To prevent %[1]sgh%[1]s from interpreting flags intended for Copilot,
			use %[1]s--%[1]s before Copilot flags and args.

			Learn more at https://gh.io/copilot-cli
		`, "`", copilotInstallDir()),
		Example: heredoc.Doc(`
			# Download and run the Copilot CLI
			$ gh copilot

			# Run the Copilot CLI
			$ gh copilot -p "Summarize this week's commits" --allow-tool 'shell(git)'

			# Remove the Copilot CLI (if installed through gh)
			$ gh copilot --remove

			# Run the Copilot CLI help command
			$ gh copilot -- --help
		`),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			stopParsePos := -1
			for i, arg := range args {
				if arg == "--" {
					stopParsePos = i
					break
				}
			}

			ghArgs := args
			opts.CopilotArgs = args
			if stopParsePos >= 0 {
				ghArgs = args[:stopParsePos]
				opts.CopilotArgs = args[stopParsePos+1:] // +1 to skip the "--" itself
			}

			if slices.Contains(ghArgs, "--help") || slices.Contains(ghArgs, "-h") {
				return cmd.Help()
			}

			if slices.Contains(ghArgs, "--remove") {
				hasOtherArgs := len(ghArgs) > 1
				if stopParsePos >= 0 {
					hasOtherArgs = hasOtherArgs || len(opts.CopilotArgs) > 0
				}
				if hasOtherArgs {
					return cmdutil.FlagErrorf("cannot use --remove with args")
				}
				opts.Remove = true
				opts.CopilotArgs = nil
			}

			if runF != nil {
				return runF(opts)
			}

			return runCopilot(opts)
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	// We add this flag, even though flag parsing is disabled for this command
	// so the flag still appears in the help text.
	cmd.Flags().Bool("remove", false, "Remove the downloaded Copilot CLI")
	return cmd
}

func runCopilot(opts *CopilotOptions) error {
	if opts.Remove {
		if err := removeCopilot(copilotInstallDir()); err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintln(opts.IO.ErrOut, "Copilot CLI removed successfully")
		}
		return nil
	}

	copilotPath := findCopilotBinary()
	if copilotPath == "" {
		if opts.IO.CanPrompt() {
			confirmed, err := opts.Prompter.Confirm("GitHub Copilot CLI is not installed. Would you like to install it?", true)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintf(opts.IO.ErrOut, "%s Copilot CLI was not installed", opts.IO.ColorScheme().WarningIcon())
				return cmdutil.SilentError
			}
		} else if !ci.IsCI() {
			fmt.Fprintf(opts.IO.ErrOut, "%s Copilot CLI not installed", opts.IO.ColorScheme().WarningIcon())
			return cmdutil.SilentError
		}

		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}

		copilotPath, err = downloadCopilot(httpClient, opts.IO, copilotInstallDir(), copilotBinaryPath())
		if err != nil {
			return err
		}
	}

	externalCmd := exec.Command(copilotPath, opts.CopilotArgs...)
	externalCmd.Stdin = opts.IO.In
	externalCmd.Stdout = opts.IO.Out
	externalCmd.Stderr = opts.IO.ErrOut
	externalCmd.Env = append(os.Environ(), "COPILOT_GH=true")

	if err := externalCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// We terminate with os.Exit here, preserving the exit code from Copilot CLI,
			// and also preventing stdio writes by callers up the stack.
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

const copilotBinaryName = "copilot"

func copilotInstallDir() string {
	return filepath.Join(config.DataDir(), "copilot")
}

func copilotBinaryPath() string {
	binaryName := copilotBinaryName
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return filepath.Join(copilotInstallDir(), binaryName)
}

// findCopilotBinary returns the path to the Copilot CLI binary, if installed,
// with the following order of precedence:
//  1. `copilot` in the PATH
//  2. `copilot` in gh's data directory
//
// If not installed, it returns an empty string.
func findCopilotBinary() string {
	if path, err := exec.LookPath(copilotBinaryName); err == nil {
		return path
	}

	localPath := copilotBinaryPath()
	if _, err := os.Stat(localPath); err != nil {
		return ""
	}
	return localPath
}

// downloadCopilot downloads and installs the Copilot CLI to installDir.
// It returns the path to the installed Copilot binary.
func downloadCopilot(httpClient *http.Client, ios *iostreams.IOStreams, installDir, localPath string) (string, error) {
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}

	if arch != "x64" && arch != "arm64" {
		return "", fmt.Errorf("unsupported architecture: %s (supported: x64, arm64)", arch)
	}

	var archiveURL string
	var archiveName string
	var isZip bool
	switch platform {
	case "win32":
		archiveName = fmt.Sprintf("copilot-%s-%s.zip", platform, arch)
		archiveURL = fmt.Sprintf("https://github.com/github/copilot-cli/releases/latest/download/%s", archiveName)
		isZip = true
	case "linux", "darwin":
		archiveName = fmt.Sprintf("copilot-%s-%s.tar.gz", platform, arch)
		archiveURL = fmt.Sprintf("https://github.com/github/copilot-cli/releases/latest/download/%s", archiveName)
	default:
		return "", fmt.Errorf("unsupported platform: %s (supported: linux, darwin, windows)", platform)
	}

	checksumsURL := "https://github.com/github/copilot-cli/releases/latest/download/SHA256SUMS.txt"

	expectedChecksum, err := fetchExpectedChecksum(httpClient, checksumsURL, archiveName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch checksums: %w", err)
	}

	ios.StartProgressIndicatorWithLabel(fmt.Sprintf("Downloading Copilot CLI from %s", archiveURL))
	defer ios.StopProgressIndicator()

	resp, err := httpClient.Get(archiveURL)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Download to temp file while calculating checksum
	tmpFile, err := os.CreateTemp("", "copilot-download-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	hasher := sha256.New()
	if _, err := io.Copy(tmpFile, io.TeeReader(resp.Body, hasher)); err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}

	ios.StopProgressIndicator()

	// Validate checksum
	actualChecksumHex := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksumHex != expectedChecksum {
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksumHex)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek temp file: %w", err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %w", err)
	}

	// Extract from the downloaded data
	if isZip {
		err = extractZip(tmpFile.Name(), installDir)
	} else {
		err = extractTarGz(tmpFile, installDir)
	}
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("copilot binary unavailable: %w", err)
	}

	fmt.Fprintf(ios.ErrOut, "%s Copilot CLI installed successfully\n", ios.ColorScheme().SuccessIcon())
	return localPath, nil
}

// fetchExpectedChecksum downloads the SHA256SUMS.txt file and returns the expected checksum for the given archive name.
func fetchExpectedChecksum(httpClient *http.Client, checksumsURL, archiveName string) (string, error) {
	resp, err := httpClient.Get(checksumsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download checksums: %s", resp.Status)
	}

	// Parse the checksums file. Possible formats are:
	// - "<checksum>  <filename>" (two whitespaces)
	// - "<checksum> <filename>"
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			checksum := fields[0]
			filename := fields[1]
			if filename == archiveName {
				return checksum, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read checksums: %w", err)
	}

	return "", fmt.Errorf("checksum not found for %s", archiveName)
}

// extractZip reads a ZIP archive at path and extracts its contents into destDir.
// It returns an error if the archive cannot be read,
// or if any file or directory within the archive cannot be created or written.
func extractZip(path, destDir string) error {
	zipReader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer zipReader.Close()

	absPath, err := safepaths.ParseAbsolute(destDir)
	if err != nil {
		return err
	}

	// As of the time of writing, ghzip.ExtractZip will safely skip files that
	// would result in path traversal. This is an issue for our use-case because
	// we want to error out before extracting if there's any such file.
	// To avoid breaking the shared ghzip.ExtractZip code that expects unsafe
	// paths to be ignored and no error produced, we pre-validate here,
	// producing an error if any such file is found.
	for _, f := range zipReader.File {
		_, err := absPath.Join(f.Name)
		if err != nil {
			return err
		}
	}

	if err := ghzip.ExtractZip(&zipReader.Reader, absPath); err != nil {
		return err
	}

	return nil
}

// extractTarGz reads a TAR.GZ archive from r and extracts its contents into destDir.
// It returns an error if the archive cannot be read,
// or if any file or directory within the archive cannot be created or written.
func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	absDestDirPath, err := safepaths.ParseAbsolute(destDir)
	if err != nil {
		return err
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		absFilePath, err := absDestDirPath.Join(header.Name)
		if err != nil {
			return err
		}
		target := absFilePath.String()

		if header.Typeflag == tar.TypeReg {
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}
			if err := extractFile(target, os.FileMode(header.Mode)&0777, tr); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractFile creates a file at target with the given mode and copies content from r.
func extractFile(target string, mode os.FileMode, r io.Reader) (err error) {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if cerr := out.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

func removeCopilot(installDir string) error {
	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		return fmt.Errorf("failed to remove Copilot CLI: Copilot CLI not installed through `gh`")
	}

	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("failed to remove Copilot CLI: %w", err)
	}

	return nil
}
