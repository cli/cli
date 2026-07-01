package readdir

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"golang.org/x/tools/txtar"
)

// isTextFile heuristically determines if a byte slice is a text file.
func isTextFile(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	// Quick check for NUL bytes, which are rare in text files
	if bytes.IndexByte(data, 0) != -1 {
		return false
	}
	// Fallback to utf8 validation for pure text (might fail on some ISO-8859-1 encodings,
	// but GitHub's source code is mostly UTF-8)
	if utf8.Valid(data) {
		return true
	}
	// Fallback to net/http's sniffer
	contentType := http.DetectContentType(data)
	return strings.HasPrefix(contentType, "text/") || contentType == "application/json" || contentType == "application/xml"
}

// writeArchive downloads the repo tarball and writes out a txtar, tar, or zip archive
func writeArchive(opts *ReadDirOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	// 1. Build the tarball URL
	apiPath := fmt.Sprintf("%srepos/%s/%s/tarball",
		ghinstance.RESTPrefix(repo.RepoHost()),
		repo.RepoOwner(), repo.RepoName(),
	)
	if opts.Ref != "" {
		apiPath += "/" + opts.Ref
	}

	req, err := http.NewRequest("GET", apiPath, nil)
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

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read tarball gzip: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	var out io.Writer
	var fOut *os.File
	if opts.Output != "" {
		fOut, err = os.Create(opts.Output)
		if err != nil {
			return err
		}
		defer fOut.Close()
		out = fOut
	} else if opts.Format == "txtar" {
		out = opts.IO.Out
	} else {
		return errors.New("output file must be specified for tar and zip formats")
	}

	var tw *tar.Writer
	var zw *zip.Writer
	var gw *gzip.Writer
	txtArchive := new(txtar.Archive)

	switch opts.Format {
	case "tar":
		if strings.HasSuffix(opts.Output, ".gz") || strings.HasSuffix(opts.Output, ".tgz") {
			gw = gzip.NewWriter(out)
			tw = tar.NewWriter(gw)
		} else {
			tw = tar.NewWriter(out)
		}
	case "zip":
		zw = zip.NewWriter(out)
	}

	// We only want paths that match dirPath (empty dirPath means all)
	// GitHub tarballs have a top-level directory like "cli-cli-1234567/".
	// We want to strip that, then check if it matches dirPath, and output it relative to dirPath.
	prefix := opts.Path
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	foundCount := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar stream: %w", err)
		}

		parts := strings.SplitN(header.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		pathInRepo := parts[1] // The path without the "owner-repo-sha" root

		if prefix != "" && !strings.HasPrefix(pathInRepo, prefix) {
			continue
		}

		// Calculate relative path to store in archive
		relPath := pathInRepo
		if prefix != "" {
			relPath = strings.TrimPrefix(pathInRepo, prefix)
		}

		if relPath == "" && header.Typeflag == tar.TypeDir {
			continue // Skip the directory itself if it's the root of the archive
		}

		foundCount++

		switch opts.Format {
		case "txtar":
			if header.Typeflag == tar.TypeReg {
				data, err := io.ReadAll(tarReader)
				if err != nil {
					return err
				}
				if isTextFile(data) {
					txtArchive.Files = append(txtArchive.Files, txtar.File{
						Name: relPath,
						Data: data,
					})
				}
			}
		case "tar":
			header.Name = relPath
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if header.Typeflag == tar.TypeReg {
				if _, err := io.Copy(tw, tarReader); err != nil {
					return err
				}
			}
		case "zip":
			if header.Typeflag == tar.TypeDir {
				// Zip directory entries should end with a slash
				if !strings.HasSuffix(relPath, "/") {
					relPath += "/"
				}
				if _, err := zw.Create(relPath); err != nil {
					return err
				}
			} else if header.Typeflag == tar.TypeReg {
				w, err := zw.Create(relPath)
				if err != nil {
					return err
				}
				if _, err := io.Copy(w, tarReader); err != nil {
					return err
				}
			}
		}
	}

	switch opts.Format {
	case "txtar":
		data := txtar.Format(txtArchive)
		if _, err := out.Write(data); err != nil {
			return err
		}
	case "tar":
		if err := tw.Close(); err != nil {
			return err
		}
		if gw != nil {
			if err := gw.Close(); err != nil {
				return err
			}
		}
	case "zip":
		if err := zw.Close(); err != nil {
			return err
		}
	}

	if foundCount == 0 {
		location := ghrepo.FullName(repo)
		if opts.Path != "" {
			location = fmt.Sprintf("%s/%s", location, strings.TrimPrefix(opts.Path, "/"))
		}
		fmt.Fprintf(opts.IO.ErrOut, "No entries found in %s\n", location)
	} else if opts.Output != "" && opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Wrote %s archive to %s\n", cs.SuccessIcon(), opts.Format, opts.Output)
	}

	return nil
}
