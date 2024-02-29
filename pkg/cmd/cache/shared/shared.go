package shared

import (
	"fmt"
	"net/url"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

var CacheFields = []string{
	"createdAt",
	"id",
	"key",
	"lastAccessedAt",
	"ref",
	"sizeInBytes",
	"version",
}

type Cache struct {
	CreatedAt      time.Time `json:"created_at"`
	Id             int       `json:"id"`
	Key            string    `json:"key"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	Ref            string    `json:"ref"`
	SizeInBytes    int64     `json:"size_in_bytes"`
	Version        string    `json:"version"`
}

type CachePayload struct {
	ActionsCaches []Cache `json:"actions_caches"`
	TotalCount    int     `json:"total_count"`
}

type GetCachesOptions struct {
	Limit int
	Order string
	Sort  string
	Key   string
	Ref   string
}

// Return a list of caches for a repository. Pass a negative limit to request
// all pages from the API until all caches have been fetched.
func GetCaches(client *api.Client, repo ghrepo.Interface, opts GetCachesOptions) (*CachePayload, error) {
	path := fmt.Sprintf("repos/%s/actions/caches", ghrepo.FullName(repo))

	perPage := 100
	if opts.Limit > 0 && opts.Limit < 100 {
		perPage = opts.Limit
	}
	path += fmt.Sprintf("?per_page=%d", perPage)

	if opts.Sort != "" {
		path += fmt.Sprintf("&sort=%s", opts.Sort)
	}
	if opts.Order != "" {
		path += fmt.Sprintf("&direction=%s", opts.Order)
	}
	if opts.Key != "" {
		path += fmt.Sprintf("&key=%s", url.QueryEscape(opts.Key))
	}
	if opts.Ref != "" {
		path += fmt.Sprintf("&ref=%s", url.QueryEscape(opts.Ref))
	}

	var result *CachePayload
pagination:
	for path != "" {
		var response CachePayload
		var err error
		path, err = client.RESTWithNext(repo.RepoHost(), "GET", path, nil, &response)
		if err != nil {
			return nil, err
		}

		if result == nil {
			result = &response
		} else {
			result.ActionsCaches = append(result.ActionsCaches, response.ActionsCaches...)
		}

		if opts.Limit > 0 && len(result.ActionsCaches) >= opts.Limit {
			result.ActionsCaches = result.ActionsCaches[:opts.Limit]
			break pagination
		}
	}

	return result, nil
}

func (c *Cache) ExportData(fields []string) map[string]interface{} {
	return cmdutil.StructExportData(c, fields)
}
