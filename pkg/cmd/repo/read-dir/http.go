package readdir

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// Git object modes. The high bits encode the object type (modeTypeMask); regular
// files additionally carry permission bits, so they are matched against modeFile
// after masking rather than by an exact value.
const (
	modeTypeMask  = 0o170000
	modeDir       = 0o040000
	modeFile      = 0o100000
	modeSymlink   = 0o120000
	modeSubmodule = 0o160000
)

// repoDir is a resolved directory listing for a single tree path.
type repoDir struct {
	GitSHA  string
	ID      string
	Entries []dirEntry
}

// dirEntry is a single entry within a directory listing.
type dirEntry struct {
	Name      string
	Path      string
	NameRaw   string
	PathRaw   string
	Type      string
	GitType   string
	Mode      int
	GitSHA    string
	Size      int
	Submodule *submodule
}

// submodule holds the extra metadata GraphQL exposes for a submodule entry.
type submodule struct {
	GitURL              string
	Branch              *string
	SubprojectCommitOid string
}

// isExecutable reports whether the entry is a regular file with an executable bit set.
func (e dirEntry) isExecutable() bool {
	return e.Type == "file" && e.Mode&0o111 != 0
}

// modeOctal renders the git mode as a six-digit octal string (e.g. 100644).
func (e dirEntry) modeOctal() string {
	return fmt.Sprintf("%06o", e.Mode)
}

// ExportData implements the cmdutil exportable interface for a single entry.
func (e dirEntry) ExportData(fields []string) map[string]interface{} {
	data := map[string]interface{}{}
	for _, field := range fields {
		switch field {
		case "name":
			data[field] = e.Name
		case "path":
			data[field] = e.Path
		case "nameRaw":
			data[field] = e.NameRaw
		case "pathRaw":
			data[field] = e.PathRaw
		case "type":
			data[field] = e.Type
		case "gitType":
			data[field] = e.GitType
		case "mode":
			data[field] = e.Mode
		case "modeOctal":
			data[field] = e.modeOctal()
		case "gitSHA":
			data[field] = e.GitSHA
		case "size":
			data[field] = e.Size
		case "submodule":
			if e.Submodule == nil {
				data[field] = nil
			} else {
				data[field] = map[string]interface{}{
					"gitUrl":              e.Submodule.GitURL,
					"branch":              e.Submodule.Branch,
					"subprojectCommitOid": e.Submodule.SubprojectCommitOid,
				}
			}
		}
	}
	return data
}

// ExportData implements the cmdutil exportable interface for the directory.
//
// gitSHA and id are structural and always present; the requested fields select
// which properties appear on each entry.
func (d *repoDir) ExportData(fields []string) map[string]interface{} {
	entries := make([]interface{}, 0, len(d.Entries))
	for _, e := range d.Entries {
		entries = append(entries, e.ExportData(fields))
	}
	return map[string]interface{}{
		"gitSHA":  d.GitSHA,
		"id":      d.ID,
		"entries": entries,
	}
}

// fetchTree resolves a directory listing for dirPath at ref.
//
// It uses the repository.object(expression) field, which returns a Tree for a
// directory, a Blob for a file, or null when the path or ref cannot be resolved.
// Unlike the REST Contents API, GraphQL has no 1000-entry cap and exposes the git
// mode, which lets us tell files, dirs, symlinks, and submodules apart.
//
// A null object means the path or ref could not be resolved (GraphQL cannot tell
// the two apart, so we report a single ambiguous error). A non-Tree object means
// the path points to a file rather than a directory. The original ref (empty for
// the default branch) is only used when building error messages.
func fetchTree(httpClient *http.Client, repo ghrepo.Interface, dirPath, ref string) (*repoDir, error) {
	client := api.NewClientFromHTTP(httpClient)

	expressionRef := ref
	if expressionRef == "" {
		expressionRef = "HEAD"
	}
	expression := fmt.Sprintf("%s:%s", expressionRef, strings.TrimPrefix(dirPath, "/"))

	var query struct {
		Repository struct {
			Object *struct {
				TypeName string `graphql:"__typename"`
				Tree     struct {
					OID     string `graphql:"oid"`
					ID      string `graphql:"id"`
					Entries []struct {
						Name      string
						Path      string
						NameRaw   string
						PathRaw   string
						Type      string
						Mode      int
						OID       string `graphql:"oid"`
						Size      int
						Submodule *struct {
							GitURL              string `graphql:"gitUrl"`
							Branch              *string
							SubprojectCommitOid string
						}
					}
				} `graphql:"... on Tree"`
			} `graphql:"object(expression: $expression)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":      githubv4.String(repo.RepoOwner()),
		"name":       githubv4.String(repo.RepoName()),
		"expression": githubv4.String(expression),
	}

	if err := client.Query(repo.RepoHost(), "RepoReadDir", &query, variables); err != nil {
		return nil, err
	}

	obj := query.Repository.Object
	if obj == nil {
		// The API returns a null object for both missing paths or refs, so we
		// cannot tell which one is wrong and infer the message from whether a ref was given.
		if ref != "" {
			return nil, fmt.Errorf("could not find %q at %q in %s (the path or ref may not exist)", dirPath, ref, ghrepo.FullName(repo))
		}
		return nil, fmt.Errorf("could not find %q in %s", dirPath, ghrepo.FullName(repo))
	}

	if obj.TypeName != "Tree" {
		if obj.TypeName == "Blob" {
			return nil, fmt.Errorf("%q is a file, not a directory; use `gh repo read-file` instead", dirPath)
		}
		return nil, fmt.Errorf("%q is not a directory", dirPath)
	}

	dir := &repoDir{
		GitSHA:  obj.Tree.OID,
		ID:      obj.Tree.ID,
		Entries: make([]dirEntry, 0, len(obj.Tree.Entries)),
	}

	for _, e := range obj.Tree.Entries {
		entry := dirEntry{
			Name:    e.Name,
			Path:    e.Path,
			NameRaw: e.NameRaw,
			PathRaw: e.PathRaw,
			Type:    entryTypeFromMode(e.Mode),
			GitType: e.Type,
			Mode:    e.Mode,
			GitSHA:  e.OID,
			Size:    e.Size,
		}
		if e.Submodule != nil {
			entry.Submodule = &submodule{
				GitURL:              e.Submodule.GitURL,
				Branch:              e.Submodule.Branch,
				SubprojectCommitOid: e.Submodule.SubprojectCommitOid,
			}
		}
		dir.Entries = append(dir.Entries, entry)
	}

	return dir, nil
}

// entryTypeFromMode normalizes a git object mode into the entry type names the
// command reports: dir, file, symlink, or submodule. Modes with an unrecognized
// type are reported as "unknown".
func entryTypeFromMode(mode int) string {
	switch mode & modeTypeMask {
	case modeDir:
		return "dir"
	case modeFile:
		return "file"
	case modeSymlink:
		return "symlink"
	case modeSubmodule:
		return "submodule"
	default:
		return "unknown"
	}
}
