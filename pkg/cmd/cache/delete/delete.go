package delete

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/cache/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	DeleteAll  bool
	Identifier string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "delete [<cache-id>| <cache-key> | --all]",
		Short: "Delete GitHub Actions caches",
		Long: heredoc.Docf(`
			Delete GitHub Actions caches.

			Deletion requires authorization with the %[1]srepo%[1]s scope.
		`, "`"),
		Example: heredoc.Doc(`
			# Delete a cache by id
			$ gh cache delete 1234

			# Delete a cache by key
			$ gh cache delete cache-key

			# Delete a cache by id in a specific repo
			$ gh cache delete 1234 --repo cli/cli

			# Delete all caches
			$ gh cache delete --all
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support -R/--repo flag
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive(
				"specify only one of cache id, cache key, or --all",
				opts.DeleteAll, len(args) > 0,
			); err != nil {
				return err
			}

			if !opts.DeleteAll && len(args) == 0 {
				return cmdutil.FlagErrorf("must provide either cache id, cache key, or use --all")
			}

			if len(args) == 1 {
				opts.Identifier = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.DeleteAll, "all", "a", false, "Delete all caches")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(httpClient)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	var toDelete []string
	if opts.DeleteAll {
		caches, err := shared.GetCaches(client, repo, shared.GetCachesOptions{Limit: -1})
		if err != nil {
			return err
		}
		if len(caches.ActionsCaches) == 0 {
			return fmt.Errorf("%s No caches to delete", opts.IO.ColorScheme().FailureIcon())
		}
		for _, cache := range caches.ActionsCaches {
			toDelete = append(toDelete, strconv.Itoa(cache.Id))
		}
	} else {
		toDelete = append(toDelete, opts.Identifier)
	}

	return deleteCaches(opts, client, repo, toDelete)
}

func deleteCaches(opts *DeleteOptions, client *api.Client, repo ghrepo.Interface, toDelete []string) error {
	cs := opts.IO.ColorScheme()
	repoName := ghrepo.FullName(repo)
	opts.IO.StartProgressIndicator()
	base := fmt.Sprintf("repos/%s/actions/caches", repoName)

	for _, cache := range toDelete {
		path := ""
		if id, err := strconv.Atoi(cache); err == nil {
			path = fmt.Sprintf("%s/%d", base, id)
		} else {
			path = fmt.Sprintf("%s?key=%s", base, url.QueryEscape(cache))
		}

		err := client.REST(repo.RepoHost(), "DELETE", path, nil, nil)
		if err != nil {
			var httpErr api.HTTPError
			if errors.As(err, &httpErr) {
				if httpErr.StatusCode == http.StatusNotFound {
					err = fmt.Errorf("%s Could not find a cache matching %s in %s", cs.FailureIcon(), cache, repoName)
				} else {
					err = fmt.Errorf("%s Failed to delete cache: %w", cs.FailureIcon(), err)
				}
			}
			opts.IO.StopProgressIndicator()
			return err
		}
	}

	opts.IO.StopProgressIndicator()

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Deleted %s from %s\n", cs.SuccessIcon(), text.Pluralize(len(toDelete), "cache"), repoName)
	}

	return nil
}
