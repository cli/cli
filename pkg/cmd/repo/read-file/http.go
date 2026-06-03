package readfile

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

// repoFile is the resolved file content and metadata for a single path.
type repoFile struct {
	Name        string
	Path        string
	SHA         string
	Size        int
	URL         string
	HTMLURL     string
	GitURL      string
	DownloadURL string
	Type        string
	Encoding    string
	Content     []byte
}

// ExportData implements the cmdutil exportable interface for --json output.
func (f *repoFile) ExportData(fields []string) map[string]interface{} {
	data := map[string]interface{}{}
	for _, field := range fields {
		switch field {
		case "name":
			data[field] = f.Name
		case "path":
			data[field] = f.Path
		case "gitSHA":
			data[field] = f.SHA
		case "size":
			data[field] = f.Size
		case "url":
			data[field] = f.URL
		case "htmlUrl":
			data[field] = f.HTMLURL
		case "gitUrl":
			data[field] = f.GitURL
		case "downloadUrl":
			data[field] = f.DownloadURL
		case "type":
			data[field] = f.Type
		case "encoding":
			data[field] = "base64"
		case "content":
			data[field] = base64.StdEncoding.EncodeToString(f.Content)
		}
	}
	return data
}

// contentsResponse models the REST Contents API response for a single path.
type contentsResponse struct {
	Type            string `json:"type"`
	Encoding        string `json:"encoding"`
	Size            int    `json:"size"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	Content         string `json:"content"`
	SHA             string `json:"sha"`
	URL             string `json:"url"`
	GitURL          string `json:"git_url"`
	HTMLURL         string `json:"html_url"`
	DownloadURL     string `json:"download_url"`
	Target          string `json:"target"`
	SubmoduleGitURL string `json:"submodule_git_url"`
}

// fetchContent retrieves the raw Contents API response for a single path.
//
// It requests the unified object media type so directories, files, symlinks, and
// submodules all come back as a single JSON object distinguished by the type field.
func fetchContent(httpClient *http.Client, repo ghrepo.Interface, filePath, ref string) (*contentsResponse, error) {
	apiPath := contentsAPIPath(repo, filePath, ref)

	req, err := http.NewRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	// We use the "application/vnd.github.object+json" media type to request a unified object
	// representation from the Contents API. Without this, the API returns a JSON array for
	// directories and a JSON object for files.
	req.Header.Set("Accept", "application/vnd.github.object+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var content contentsResponse
	if err := json.Unmarshal(body, &content); err != nil {
		return nil, err
	}
	return &content, nil
}

// fetchFile retrieves a single path's metadata and inline content via the REST Contents API.
//
// It returns a typed error when the path is a directory, symlink, or submodule. Content is
// populated only when the API returns it inline; larger files come back with empty content, and
// it is up to the caller to fetch the raw bytes via fetchRawFile when the content is actually
// needed.
func fetchFile(httpClient *http.Client, repo ghrepo.Interface, filePath, ref string) (*repoFile, error) {
	content, err := fetchContent(httpClient, repo, filePath, ref)
	if err != nil {
		return nil, err
	}

	if content.Type != "file" {
		// The path resolved to something other than a regular file. Use content.Path
		// (the API-sanitized path) rather than the user input in these messages, so a
		// crafted path cannot smuggle terminal escape sequences into our output.
		switch content.Type {
		case "dir":
			return nil, fmt.Errorf("path %q is a directory; use `gh repo read-dir` instead", content.Path)
		case "symlink":
			return nil, fmt.Errorf("path %q is a symlink to %q which does not exist", content.Path, content.Target)
		case "submodule":
			return nil, fmt.Errorf("path %q is a submodule (%s at %s)", content.Path, content.SubmoduleGitURL, content.SHA)
		default:
			return nil, fmt.Errorf("path %q is not a regular file (type: %s)", content.Path, content.Type)
		}
	}

	file := &repoFile{
		Name:        content.Name,
		Path:        content.Path,
		SHA:         content.SHA,
		Size:        content.Size,
		URL:         content.URL,
		HTMLURL:     content.HTMLURL,
		GitURL:      content.GitURL,
		DownloadURL: content.DownloadURL,
		Type:        content.Type,
		Encoding:    content.Encoding,
	}

	if content.Encoding == "base64" && content.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(content.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 file content: %w", err)
		}
		file.Content = decoded
	}

	return file, nil
}

// fetchRawFile retrieves the raw bytes of a file, used for files larger than the
// 1 MB inline content limit of the Contents API.
func fetchRawFile(httpClient *http.Client, repo ghrepo.Interface, filePath, ref string) ([]byte, error) {
	apiPath := contentsAPIPath(repo, filePath, ref)

	req, err := http.NewRequest("GET", apiPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	return io.ReadAll(resp.Body)
}

// contentsAPIPath builds the absolute Contents API URL for a path and optional ref.
func contentsAPIPath(repo ghrepo.Interface, filePath, ref string) string {
	// The Contents API accepts a fully percent-encoded path, including path separators
	// encoded as %2F, so spaces and other special characters are handled transparently.
	p := fmt.Sprintf("%srepos/%s/%s/contents/%s",
		ghinstance.RESTPrefix(repo.RepoHost()),
		repo.RepoOwner(), repo.RepoName(),
		url.PathEscape(strings.TrimPrefix(filePath, "/")),
	)
	if ref != "" {
		p += "?ref=" + url.QueryEscape(ref)
	}
	return p
}
