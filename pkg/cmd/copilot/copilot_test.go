package copilot

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCopilot(t *testing.T) {
	tests := []struct {
		name          string
		args          string
		wantOpts      CopilotOptions
		wantErrString string
		wantHelp      bool
	}{
		{
			name: "no argument",
			args: "",
			wantOpts: CopilotOptions{
				CopilotArgs: []string{},
			},
			wantErrString: "",
		},
		{
			name: "with arguments",
			args: "some-arg some-other-arg",
			wantOpts: CopilotOptions{
				CopilotArgs: []string{"some-arg", "some-other-arg"},
			},
		},
		{
			name: "with --remove alone",
			args: "--remove",
			wantOpts: CopilotOptions{
				Remove: true,
			},
		},
		{
			name: "with non-gh flags passed to copilot",
			args: "-p testing --something-flag",
			wantOpts: CopilotOptions{
				CopilotArgs: []string{"-p", "testing", "--something-flag"},
			},
		},
		{
			name:          "with --remove and arguments",
			args:          "--remove some-arg",
			wantErrString: "cannot use --remove with args",
		},
		{
			name: "with --remove passed to copilot using --",
			args: "-- --remove",
			wantOpts: CopilotOptions{
				CopilotArgs: []string{"--remove"},
			},
		},
		{
			name: "with --remove and -- alone",
			args: "--remove --",
			wantOpts: CopilotOptions{
				Remove: true,
			},
		},
		{
			name:          "with --remove, some invalid arg, and --",
			args:          "--remove invalid-arg --",
			wantErrString: "cannot use --remove with args",
		},
		{
			name:          "with --remove and -- and random arguments",
			args:          "--remove -- some-arg",
			wantErrString: "cannot use --remove with args",
		},
		{
			name:          "with --help, shows gh help",
			args:          "--help",
			wantErrString: "",
			wantHelp:      true,
		},
		{
			name: "with --help and --, shows copilot help",
			args: "-- --help",
			wantOpts: CopilotOptions{
				CopilotArgs: []string{"--help"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var gotOpts *CopilotOptions
			cmd := NewCmdCopilot(f, func(opts *CopilotOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErrString != "" {
				require.EqualError(t, err, tt.wantErrString)
				return
			}

			if tt.wantHelp {
				require.NoError(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.CopilotArgs, gotOpts.CopilotArgs, "opts.CopilotArgs not as expected")
			assert.Equal(t, tt.wantOpts.Remove, gotOpts.Remove, "opts.Remove not as expected")
		})
	}
}

func TestRemoveCopilot(t *testing.T) {
	t.Run("removes existing install directory", func(t *testing.T) {
		// Create a temporary directory to simulate the install directory
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		require.NoError(t, os.MkdirAll(installDir, 0755), "failed to create test directory")
		// Create a dummy file in the directory
		dummyFile := filepath.Join(installDir, "copilot")
		require.NoError(t, os.WriteFile(dummyFile, []byte("test"), 0755), "failed to create test file")

		err := removeCopilot(installDir)
		require.NoError(t, err, "unexpected error")

		_, err = os.Stat(installDir)
		require.True(t, os.IsNotExist(err), "expected install directory to be removed")
	})

	t.Run("handles non-existent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")

		require.ErrorContains(t, removeCopilot(installDir), "failed to remove Copilot CLI")
	})
}

// createTarGzBuffer creates a tar.gz archive in memory with the given files.
func createTarGzBuffer(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0755,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(hdr), "failed to write tar header")
		_, err := tw.Write(content)
		require.NoError(t, err, "failed to write tar content")
	}

	require.NoError(t, tw.Close(), "failed to close tar writer")
	require.NoError(t, gw.Close(), "failed to close gzip writer")
	return buf.Bytes()
}

// createZipBuffer creates a zip archive in memory with the given files.
func createZipBuffer(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range files {
		fw, err := zw.Create(name)
		require.NoError(t, err, "failed to create zip entry")
		_, err = fw.Write(content)
		require.NoError(t, err, "failed to write zip content")
	}

	require.NoError(t, zw.Close(), "failed to close zip writer")
	return buf.Bytes()
}

func TestExtractTarGz(t *testing.T) {
	t.Run("extracts files correctly", func(t *testing.T) {
		content := []byte("hello world")
		archive := createTarGzBuffer(t, map[string][]byte{
			"copilot": content,
		})

		destDir := t.TempDir()

		err := extractTarGz(bytes.NewReader(archive), destDir)
		require.NoError(t, err, "extractTarGz() error")

		extracted, err := os.ReadFile(filepath.Join(destDir, "copilot"))
		require.NoError(t, err, "failed to read extracted file")
		require.Equal(t, content, extracted, "extracted content mismatch")
	})

	t.Run("extracts nested files", func(t *testing.T) {
		content := []byte("nested content")
		archive := createTarGzBuffer(t, map[string][]byte{
			"subdir/file.txt": content,
		})

		destDir := t.TempDir()

		err := extractTarGz(bytes.NewReader(archive), destDir)
		require.NoError(t, err, "extractTarGz() error")

		extracted, err := os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))
		require.NoError(t, err, "failed to read extracted file")
		require.Equal(t, content, extracted, "extracted content mismatch")
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		// Manually create a malicious tar.gz with path traversal
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)

		hdr := &tar.Header{
			Name: "../evil.txt",
			Mode: 0755,
			Size: 4,
		}
		_ = tw.WriteHeader(hdr)
		_, _ = tw.Write([]byte("evil"))
		_ = tw.Close()
		_ = gw.Close()

		destDir := t.TempDir()

		err := extractTarGz(bytes.NewReader(buf.Bytes()), destDir)
		require.Error(t, err, "expected error for path traversal, got nil")
	})

	t.Run("handles invalid gzip", func(t *testing.T) {
		destDir := t.TempDir()

		err := extractTarGz(bytes.NewReader([]byte("not valid gzip")), destDir)
		require.Error(t, err, "expected error for invalid gzip, got nil")
	})
}

func TestExtractZip(t *testing.T) {
	t.Run("extracts files correctly", func(t *testing.T) {
		zipDir := t.TempDir()
		zipPath := filepath.Join(zipDir, "archive.zip")
		content := []byte("hello world")
		archive := createZipBuffer(t, map[string][]byte{
			"copilot.exe": content,
		})
		require.NoError(t, os.WriteFile(zipPath, archive, 0x755))

		destDir := t.TempDir()

		err := extractZip(zipPath, destDir)
		require.NoError(t, err, "extractZip() error")

		extracted, err := os.ReadFile(filepath.Join(destDir, "copilot.exe"))
		require.NoError(t, err, "failed to read extracted file")
		require.Equal(t, content, extracted, "extracted content mismatch")
	})

	t.Run("extracts nested files", func(t *testing.T) {
		zipDir := t.TempDir()
		zipPath := filepath.Join(zipDir, "archive.zip")
		content := []byte("hello world")
		archive := createZipBuffer(t, map[string][]byte{
			"subdir/file.txt": content,
		})
		require.NoError(t, os.WriteFile(zipPath, archive, 0x755))

		destDir := t.TempDir()

		err := extractZip(zipPath, destDir)
		require.NoError(t, err, "extractZip() error")

		extracted, err := os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))
		require.NoError(t, err, "failed to read extracted file")
		require.Equal(t, content, extracted, "extracted content mismatch")
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		zipDir := t.TempDir()
		zipPath := filepath.Join(zipDir, "archive.zip")

		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)

		fh := &zip.FileHeader{
			Name:   "../evil.txt",
			Method: zip.Store,
		}
		fw, _ := zw.CreateHeader(fh)
		_, _ = fw.Write([]byte("evil"))
		_ = zw.Close()

		require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0x755))
		destDir := t.TempDir()

		err := extractZip(zipPath, destDir)
		require.Error(t, err, "expected error for path traversal, got nil")
	})
}

func TestFetchExpectedChecksum(t *testing.T) {
	t.Run("parses checksums file correctly", func(t *testing.T) {
		reg := &httpmock.Registry{}
		checksums := "abc123def456  copilot-linux-x64.tar.gz\n789xyz  copilot-darwin-arm64.tar.gz\n"
		reg.Register(
			httpmock.MatchAny,
			httpmock.StringResponse(checksums),
		)

		client := &http.Client{Transport: reg}
		checksum, err := fetchExpectedChecksum(client, "https://example.com/checksums", "copilot-linux-x64.tar.gz")
		require.NoError(t, err, "unexpected error")
		require.Equal(t, "abc123def456", checksum, "checksum mismatch")
	})

	t.Run("returns error for missing archive", func(t *testing.T) {
		reg := &httpmock.Registry{}
		checksums := "abc123  copilot-linux-x64.tar.gz\n"
		reg.Register(
			httpmock.MatchAny,
			httpmock.StringResponse(checksums),
		)

		client := &http.Client{Transport: reg}
		_, err := fetchExpectedChecksum(client, "https://example.com/checksums", "copilot-win32-x64.zip")
		require.Error(t, err, "expected error for missing archive")
		require.Equal(t, "checksum not found for copilot-win32-x64.zip", err.Error(), "unexpected error")
	})

	t.Run("handles single space separator", func(t *testing.T) {
		reg := &httpmock.Registry{}
		checksums := "abc123 copilot-darwin-x64.tar.gz\n"
		reg.Register(
			httpmock.MatchAny,
			httpmock.StringResponse(checksums),
		)

		client := &http.Client{Transport: reg}
		checksum, err := fetchExpectedChecksum(client, "https://example.com/checksums", "copilot-darwin-x64.tar.gz")
		require.NoError(t, err, "unexpected error")
		require.Equal(t, "abc123", checksum, "checksum mismatch")
	})

	t.Run("handles HTTP error", func(t *testing.T) {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.MatchAny,
			httpmock.StatusStringResponse(http.StatusNotFound, "not found"),
		)

		client := &http.Client{Transport: reg}
		_, err := fetchExpectedChecksum(client, "https://example.com/checksums", "copilot-linux-x64.tar.gz")
		require.Error(t, err, "expected error for HTTP 404")
	})
}

func archString() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		return "x64"
	}
	return arch
}

func TestDownloadCopilot(t *testing.T) {
	// Skip on unsupported architectures
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("skipping test on unsupported architecture")
	}

	t.Run("downloads and extracts tar.gz with valid checksum", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping tar.gz test on windows")
		}

		ios, _, _, stderr := iostreams.Test()
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		localPath := filepath.Join(installDir, "copilot")

		// Create mock archive with copilot binary
		binaryContent := []byte("#!/bin/sh\necho copilot")
		archive := createTarGzBuffer(t, map[string][]byte{
			"copilot": binaryContent,
		})

		// Calculate checksum
		checksum := sha256.Sum256(archive)
		checksumHex := hex.EncodeToString(checksum[:])
		archiveName := fmt.Sprintf("copilot-%s-%s.tar.gz", runtime.GOOS, archString())
		checksumFile := fmt.Sprintf("%s  %s\n", checksumHex, archiveName)

		reg := &httpmock.Registry{}
		// Register checksum endpoint
		reg.Register(
			httpmock.REST("GET", "github/copilot-cli/releases/latest/download/SHA256SUMS.txt"),
			httpmock.StringResponse(checksumFile),
		)
		// Register archive endpoint
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("github/copilot-cli/releases/latest/download/%s", archiveName)),
			httpmock.BinaryResponse(archive),
		)

		httpClient := &http.Client{Transport: reg}

		path, err := downloadCopilot(httpClient, ios, installDir, localPath)
		require.NoError(t, err, "downloadCopilot() error")
		require.Equal(t, localPath, path, "downloadCopilot() path mismatch")

		// Verify binary was extracted
		extracted, err := os.ReadFile(localPath)
		require.NoError(t, err, "failed to read extracted binary")
		require.Equal(t, binaryContent, extracted, "extracted content mismatch")

		// Verify output messages
		require.Contains(t, stderr.String(), "installed successfully", "expected success message in stderr")
	})

	t.Run("fails with checksum mismatch", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping tar.gz test on windows")
		}

		ios, _, _, _ := iostreams.Test()
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		localPath := filepath.Join(installDir, "copilot")

		binaryContent := []byte("#!/bin/sh\necho copilot")
		archive := createTarGzBuffer(t, map[string][]byte{
			"copilot": binaryContent,
		})

		// Use wrong checksum
		archiveName := fmt.Sprintf("copilot-%s-%s.tar.gz", runtime.GOOS, archString())
		checksumFile := fmt.Sprintf("%s  %s\n", "0000000000000000000000000000000000000000000000000000000000000000", archiveName)

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "github/copilot-cli/releases/latest/download/SHA256SUMS.txt"),
			httpmock.StringResponse(checksumFile),
		)
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("github/copilot-cli/releases/latest/download/%s", archiveName)),
			httpmock.BinaryResponse(archive),
		)

		httpClient := &http.Client{Transport: reg}

		_, err := downloadCopilot(httpClient, ios, installDir, localPath)
		require.Error(t, err, "expected error for checksum mismatch, got nil")
		require.Contains(t, err.Error(), "checksum mismatch", "expected checksum mismatch error")
	})

	t.Run("handles HTTP error on archive download", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping tar.gz test on windows")
		}

		ios, _, _, _ := iostreams.Test()
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		localPath := filepath.Join(installDir, "copilot")

		archiveName := fmt.Sprintf("copilot-%s-%s.tar.gz", runtime.GOOS, archString())
		checksumFile := fmt.Sprintf("%s  %s\n", "abc123", archiveName)

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "github/copilot-cli/releases/latest/download/SHA256SUMS.txt"),
			httpmock.StringResponse(checksumFile),
		)
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("github/copilot-cli/releases/latest/download/%s", archiveName)),
			httpmock.StatusStringResponse(http.StatusNotFound, "not found"),
		)

		httpClient := &http.Client{Transport: reg}

		_, err := downloadCopilot(httpClient, ios, installDir, localPath)
		require.Error(t, err, "expected error for HTTP 404, got nil")
		require.Contains(t, err.Error(), "download failed", "expected error to contain 'download failed'")
	})

	t.Run("handles missing binary after extraction", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping tar.gz test on windows")
		}

		ios, _, _, _ := iostreams.Test()
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		localPath := filepath.Join(installDir, "copilot")

		// Create archive without the expected binary name
		archive := createTarGzBuffer(t, map[string][]byte{
			"wrong-name": []byte("content"),
		})

		checksum := sha256.Sum256(archive)
		checksumHex := hex.EncodeToString(checksum[:])
		archiveName := fmt.Sprintf("copilot-%s-%s.tar.gz", runtime.GOOS, archString())
		checksumFile := fmt.Sprintf("%s  %s\n", checksumHex, archiveName)

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "github/copilot-cli/releases/latest/download/SHA256SUMS.txt"),
			httpmock.StringResponse(checksumFile),
		)
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("github/copilot-cli/releases/latest/download/%s", archiveName)),
			httpmock.BinaryResponse(archive),
		)

		httpClient := &http.Client{Transport: reg}

		_, err := downloadCopilot(httpClient, ios, installDir, localPath)
		assert.ErrorContains(t, err, "copilot binary unavailable")
	})

	t.Run("downloads and extracts zip on windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("skipping zip test on non-windows")
		}

		ios, _, _, _ := iostreams.Test()
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "copilot")
		localPath := filepath.Join(installDir, "copilot.exe")

		binaryContent := []byte("MZ fake exe content")
		archive := createZipBuffer(t, map[string][]byte{
			"copilot.exe": binaryContent,
		})

		checksum := sha256.Sum256(archive)
		checksumHex := hex.EncodeToString(checksum[:])
		archiveName := fmt.Sprintf("copilot-%s-%s.zip", "win32", archString())
		checksumFile := fmt.Sprintf("%s  %s\n", checksumHex, archiveName)

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "github/copilot-cli/releases/latest/download/SHA256SUMS.txt"),
			httpmock.StringResponse(checksumFile),
		)
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("github/copilot-cli/releases/latest/download/%s", archiveName)),
			httpmock.BinaryResponse(archive),
		)

		httpClient := &http.Client{Transport: reg}

		path, err := downloadCopilot(httpClient, ios, installDir, localPath)
		require.NoError(t, err, "downloadCopilot() error")
		require.Equal(t, localPath, path, "downloadCopilot() path mismatch")
	})
}
