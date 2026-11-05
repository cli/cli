package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// schemaSource is one schema file that GitHub publishes. validate rejects a
// download that can't be used, such as an error page.
type schemaSource struct {
	label    string
	envVar   string
	url      string
	fileName string
	validate func(data []byte) error
}

var (
	graphQLSource = schemaSource{
		label:    "GraphQL schema",
		envVar:   "GHMOCK_GRAPHQL_SCHEMA",
		url:      "https://docs.github.com/public/fpt/schema.docs.graphql",
		fileName: "schema.docs.graphql",
		validate: func(data []byte) error {
			_, err := loadGraphQLSchema(data, "download")
			return err
		},
	}
	restSource = schemaSource{
		label:    "REST description",
		envVar:   "GHMOCK_REST_SCHEMA",
		url:      "https://raw.githubusercontent.com/github/rest-api-description/main/descriptions/api.github.com/api.github.com.json",
		fileName: "api.github.com.json",
		validate: func(data []byte) error {
			_, err := loadRESTIndex(data)
			return err
		},
	}
)

// userCacheDir and downloadClient are variables so tests can keep downloads local.
var (
	userCacheDir   = os.UserCacheDir
	downloadClient = &http.Client{Timeout: 5 * time.Minute}
)

const maxSchemaSize = 200 << 20

func schemaCacheDir() (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", fmt.Errorf("finding the cache folder: %w", err)
	}
	return filepath.Join(base, "ghmock"), nil
}

// locate returns the schema file to use: the file named by the environment
// variable, a cached copy, or a fresh download into the cache.
func (s schemaSource) locate(progress io.Writer) (string, error) {
	if p := os.Getenv(s.envVar); p != "" {
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("%s: %w", s.envVar, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s: %s is a directory", s.envVar, p)
		}
		return p, nil
	}
	dir, err := schemaCacheDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, s.fileName)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return s.download(dir, progress)
}

// download fetches the file into dir. It checks the file and writes it to a
// temporary file first, so a failed download never replaces a good copy.
func (s schemaSource) download(dir string, progress io.Writer) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(progress, "ghmock: downloading GitHub's %s to %s\n", s.label, dir)

	req, err := http.NewRequest(http.MethodGet, s.url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ghmock")
	resp, err := downloadClient.Do(req)
	if err != nil {
		return "", s.downloadError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", s.downloadError(fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSchemaSize+1))
	switch {
	case err != nil:
		return "", s.downloadError(err)
	case len(data) > maxSchemaSize:
		return "", s.downloadError(fmt.Errorf("file is larger than %d MB", maxSchemaSize>>20))
	case len(data) == 0:
		return "", s.downloadError(errors.New("file is empty"))
	}
	if err := s.validate(data); err != nil {
		return "", s.downloadError(fmt.Errorf("not a usable %s: %w", s.label, err))
	}

	tmp, err := os.CreateTemp(dir, s.fileName+".*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		// CreateTemp makes files only their owner can read. A sandboxed
		// agent may run as another user.
		err = tmp.Chmod(0o644)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, s.fileName)
	if err := os.Rename(tmp.Name(), p); err != nil {
		return "", err
	}
	return p, nil
}

func (s schemaSource) downloadError(err error) error {
	return fmt.Errorf("downloading GitHub's %s from %s: %w (without network access, set %s to a local copy)",
		s.label, s.url, err, s.envVar)
}
