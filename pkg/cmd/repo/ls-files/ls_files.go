package lsfiles

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type LsFilesOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Exporter   cmdutil.Exporter

	Path string
	Ref  string
}

type TreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int    `json:"size,omitempty"`
}

var treeEntryFields = []string{"name", "path", "type", "size"}

func NewCmdLsFiles(f *cmdutil.Factory, runF func(*LsFilesOptions) error) *cobra.Command {
	opts := &LsFilesOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "ls-files [<path>]",
		Short: "List files in a repository",
		Long: heredoc.Docf(`
			List the files and directories at a given path in a GitHub repository
			without cloning it.

			By default, lists the root of the default branch of the repository
			associated with the current directory.

			Use %[1]s-R%[1]s to specify a different repository and %[1]s--ref%[1]s to
			specify a branch, tag, or commit SHA.
		`, "`"),
		Example: heredoc.Doc(`
			# List files in the root of the current repository
			$ gh repo ls-files

			# List files in a subdirectory
			$ gh repo ls-files src/

			# List files from a specific branch
			$ gh repo ls-files --ref main

			# List files from another repository as JSON
			$ gh repo ls-files -R cli/cli --json name,type

			# List files from a specific directory and branch
			$ gh repo ls-files pkg/cmd --ref v2.50.0 -R cli/cli
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Path = args[0]
			}

			if repoOverride, _ := c.Flags().GetString("repo"); repoOverride != "" {
				opts.BaseRepo = cmdutil.OverrideBaseRepoFunc(f.BaseRepo, repoOverride)
			}

			if runF != nil {
				return runF(opts)
			}
			return lsFilesRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Ref, "ref", "", "The branch, tag, or commit SHA to list files from")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, treeEntryFields)

	return cmd
}

func lsFilesRun(opts *LsFilesOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	ref := opts.Ref
	if ref == "" {
		ref = "HEAD"
	}

	expression := ref + ":"
	if opts.Path != "" {
		expression = ref + ":" + opts.Path
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	var result struct {
		Repository struct {
			Object *struct {
				Typename string `json:"__typename"`
				Entries  []struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Path   string `json:"path"`
					Object *struct {
						ByteSize int `json:"byteSize"`
					} `json:"object"`
				} `json:"entries"`
			}
		}
	}

	query := `query RepositoryLsFiles($owner: String!, $name: String!, $expression: String!) {
		repository(owner: $owner, name: $name) {
			object(expression: $expression) {
				__typename
				... on Tree {
					entries {
						name
						type
						path
						object {
							... on Blob { byteSize }
						}
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"owner":      repo.RepoOwner(),
		"name":       repo.RepoName(),
		"expression": expression,
	}

	err = apiClient.GraphQL(repo.RepoHost(), query, variables, &result)
	if err != nil {
		return err
	}

	obj := result.Repository.Object
	if obj == nil {
		path := opts.Path
		if path == "" {
			path = "/"
		}
		return fmt.Errorf("path not found: %s (ref: %s)", path, ref)
	}

	if obj.Typename != "Tree" {
		return fmt.Errorf("path is a file, not a directory: %s (use `gh repo view-file` to view file contents)", opts.Path)
	}

	entries := make([]TreeEntry, 0, len(obj.Entries))
	for _, e := range obj.Entries {
		entry := TreeEntry{
			Name: e.Name,
			Path: e.Path,
			Type: e.Type,
		}
		if e.Object != nil {
			entry.Size = e.Object.ByteSize
		}
		entries = append(entries, entry)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, entries)
	}

	for _, e := range entries {
		prefix := ""
		if e.Type == "tree" {
			prefix = "d "
		} else {
			prefix = "  "
		}
		fmt.Fprintf(opts.IO.Out, "%s%s\n", prefix, e.Name)
	}

	return nil
}

// Implement cmdutil.Exportable for []TreeEntry
func (e TreeEntry) ExportData(fields []string) map[string]interface{} {
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "name":
			data["name"] = e.Name
		case "path":
			data["path"] = e.Path
		case "type":
			data["type"] = e.Type
		case "size":
			data["size"] = e.Size
		}
	}
	return data
}

// MarshalJSON for TreeEntry.
func (e TreeEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
		Size int    `json:"size,omitempty"`
	}{
		Name: e.Name,
		Path: e.Path,
		Type: e.Type,
		Size: e.Size,
	})
}
