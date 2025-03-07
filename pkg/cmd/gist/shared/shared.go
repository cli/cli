package shared

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/gabriel-vasile/mimetype"
	"github.com/shurcooL/githubv4"
)

type GistFile struct {
	Filename string `json:"filename,omitempty"`
	Type     string `json:"type,omitempty"`
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
}

type GistOwner struct {
	Login string `json:"login,omitempty"`
}

type Gist struct {
	ID          string               `json:"id,omitempty"`
	Description string               `json:"description"`
	Files       map[string]*GistFile `json:"files"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Public      bool                 `json:"public"`
	HTMLURL     string               `json:"html_url,omitempty"`
	Owner       *GistOwner           `json:"owner,omitempty"`
}

func (g Gist) Filename() string {
	filenames := make([]string, 0, len(g.Files))
	for fn := range g.Files {
		filenames = append(filenames, fn)
	}
	sort.Strings(filenames)
	return filenames[0]
}

func (g Gist) TruncDescription() string {
	return text.Truncate(100, text.RemoveExcessiveWhitespace(g.Description))
}

var NotFoundErr = errors.New("not found")

func GetGist(client *http.Client, hostname, gistID string) (*Gist, error) {
	gist := Gist{}
	path := fmt.Sprintf("gists/%s", gistID)

	apiClient := api.NewClientFromHTTP(client)
	err := apiClient.REST(hostname, "GET", path, nil, &gist)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return nil, NotFoundErr
		}
		return nil, err
	}

	return &gist, nil
}

func GistIDFromURL(gistURL string) (string, error) {
	u, err := url.Parse(gistURL)
	if err == nil && strings.HasPrefix(u.Path, "/") {
		split := strings.Split(u.Path, "/")

		if len(split) > 2 {
			return split[2], nil
		}

		if len(split) == 2 && split[1] != "" {
			return split[1], nil
		}
	}

	return "", fmt.Errorf("Invalid gist URL %s", u)
}

const maxPerPage = 100

func ListGists(client *http.Client, hostname string, limit int, filter *regexp.Regexp, includeContent bool, visibility string) ([]Gist, error) {
	type response struct {
		Viewer struct {
			Gists struct {
				Nodes []struct {
					Description string
					Files       []struct {
						Name string
						Text string `graphql:"text @include(if: $includeContent)"`
					}
					IsPublic  bool
					Name      string
					UpdatedAt time.Time
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"gists(first: $per_page, after: $endCursor, privacy: $visibility, orderBy: {field: CREATED_AT, direction: DESC})"`
		}
	}

	perPage := limit
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	variables := map[string]interface{}{
		"per_page":       githubv4.Int(perPage),
		"endCursor":      (*githubv4.String)(nil),
		"visibility":     githubv4.GistPrivacy(strings.ToUpper(visibility)),
		"includeContent": githubv4.Boolean(includeContent),
	}

	filterFunc := func(gist *Gist) bool {
		if filter.MatchString(gist.Description) {
			return true
		}

		for _, file := range gist.Files {
			if filter.MatchString(file.Filename) {
				return true
			}

			if includeContent && filter.MatchString(file.Content) {
				return true
			}
		}

		return false
	}

	gql := api.NewClientFromHTTP(client)

	gists := []Gist{}
pagination:
	for {
		var result response
		err := gql.Query(hostname, "GistList", &result, variables)
		if err != nil {
			return nil, err
		}

		for _, gist := range result.Viewer.Gists.Nodes {
			files := map[string]*GistFile{}
			for _, file := range gist.Files {
				files[file.Name] = &GistFile{
					Filename: file.Name,
					Content:  file.Text,
				}
			}

			gist := Gist{
				ID:          gist.Name,
				Description: gist.Description,
				Files:       files,
				UpdatedAt:   gist.UpdatedAt,
				Public:      gist.IsPublic,
			}

			if filter == nil || filterFunc(&gist) {
				gists = append(gists, gist)
			}

			if len(gists) == limit {
				break pagination
			}
		}

		if !result.Viewer.Gists.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.Viewer.Gists.PageInfo.EndCursor)
	}

	return gists, nil
}

func IsBinaryFile(file string) (bool, error) {
	detectedMime, err := mimetype.DetectFile(file)
	if err != nil {
		return false, err
	}

	isBinary := true
	for mime := detectedMime; mime != nil; mime = mime.Parent() {
		if mime.Is("text/plain") {
			isBinary = false
			break
		}
	}
	return isBinary, nil
}

func IsBinaryContents(contents []byte) bool {
	isBinary := true
	for mime := mimetype.Detect(contents); mime != nil; mime = mime.Parent() {
		if mime.Is("text/plain") {
			isBinary = false
			break
		}
	}
	return isBinary
}

func PromptGists(prompter prompter.Prompter, client *http.Client, host string, cs *iostreams.ColorScheme) (gist *Gist, err error) {
	gists, err := ListGists(client, host, 10, nil, false, "all")
	if err != nil {
		return &Gist{}, err
	}

	if len(gists) == 0 {
		return &Gist{}, nil
	}

	var opts = make([]string, len(gists))

	for i, gist := range gists {
		gistTime := text.FuzzyAgo(time.Now(), gist.UpdatedAt)
		// TODO: support dynamic maxWidth
		opts[i] = fmt.Sprintf("%s %s %s", cs.Bold(gist.Filename()), gist.TruncDescription(), cs.Gray(gistTime))
	}

	result, err := prompter.Select("Select a gist", "", opts)

	if err != nil {
		return &Gist{}, err
	}

	return &gists[result], nil
}
