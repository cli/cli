package shared

import (
	"strconv"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
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
	Id             int64     `json:"id"`
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
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "actions", "caches")
	if err != nil {
		return nil, err
	}

	perPage := 100
	if opts.Limit > 0 && opts.Limit < 100 {
		perPage = opts.Limit
	}
	u.SetQuery("per_page", strconv.Itoa(perPage))

	if opts.Sort != "" {
		u.SetQuery("sort", opts.Sort)
	}
	if opts.Order != "" {
		u.SetQuery("direction", opts.Order)
	}
	if opts.Key != "" {
		u.SetQuery("key", opts.Key)
	}
	if opts.Ref != "" {
		u.SetQuery("ref", opts.Ref)
	}
	var pageURL safeurl.SafeURL = u

	var result *CachePayload
pagination:
	for pageURL.String() != "" {
		var response CachePayload
		next, err := client.RESTWithNext(repo.RepoHost(), "GET", pageURL.String(), nil, &response)
		if err != nil {
			return nil, err
		}
		pageURL = safeurl.NewImmutableSafeURL(next)

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

func (c *Cache) ExportData(fields []string) map[string]any {
	return cmdutil.StructExportData(c, fields)
}
