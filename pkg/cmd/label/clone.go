package label

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type cloneOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	SourceRepo ghrepo.Interface
	Force      bool
}

func newCmdClone(f *cmdutil.Factory, runF func(*cloneOptions) error) *cobra.Command {
	opts := cloneOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "clone <source-repository>",
		Short: "Clones labels from one repository to another",
		Long: heredoc.Doc(`
			Clones labels from a source repository to a destination repository on GitHub.
			By default, the destination repository is the current repository.

			All labels from the source repository will be copied to the destination
			repository. Labels in the destination repository that are not in the source
			repository will not be deleted or modified.

			Labels from the source repository that already exist in the destination
			repository will be skipped. You can overwrite existing labels in the
			destination repository using the --force flag.
		`),
		Example: heredoc.Doc(`
			# clone and overwrite labels from cli/cli repository into the current repository
			$ gh label clone cli/cli --force

			# clone labels from cli/cli repository into a octocat/cli repository
			$ gh label clone cli/cli --repo octocat/cli
		`),
		Args: cmdutil.ExactArgs(1, "cannot clone labels: source-repository argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			var err error
			opts.BaseRepo = f.BaseRepo
			opts.SourceRepo, err = ghrepo.FromFullName(args[0])
			if err != nil {
				return err
			}
			if runF != nil {
				return runF(&opts)
			}
			return cloneRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Overwrite labels in the destination repository")

	return cmd
}

func cloneRun(opts *cloneOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	successCount, totalCount, err := cloneLabels(httpClient, baseRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		pluralize := func(num int) string {
			return text.Pluralize(num, "label")
		}

		successCount := int(successCount)
		switch {
		case successCount == totalCount:
			fmt.Fprintf(opts.IO.Out, "%s Cloned %s from %s to %s\n", cs.SuccessIcon(), pluralize(successCount), ghrepo.FullName(opts.SourceRepo), ghrepo.FullName(baseRepo))
		default:
			fmt.Fprintf(opts.IO.Out, "%s Cloned %s of %d from %s to %s\n", cs.WarningIcon(), pluralize(successCount), totalCount, ghrepo.FullName(opts.SourceRepo), ghrepo.FullName(baseRepo))
		}
	}

	return nil
}

func cloneLabels(client *http.Client, destination ghrepo.Interface, opts *cloneOptions) (successCount uint32, totalCount int, err error) {
	successCount = 0
	labels, totalCount, err := listLabels(client, opts.SourceRepo, listQueryOptions{Limit: -1})
	if err != nil {
		return
	}

	workers := 10
	toCreate := make(chan createOptions)

	wg, ctx := errgroup.WithContext(context.Background())
	for i := 0; i < workers; i++ {
		wg.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return nil
				case l, ok := <-toCreate:
					if !ok {
						return nil
					}
					err := createLabel(client, destination, &l)
					if err != nil {
						if !errors.Is(err, errLabelAlreadyExists) {
							return err
						}
					} else {
						atomic.AddUint32(&successCount, 1)
					}
				}
			}
		})
	}

	for _, label := range labels {
		createOpts := createOptions{
			Name:        label.Name,
			Description: label.Description,
			Color:       label.Color,
			Force:       opts.Force,
		}
		toCreate <- createOpts
	}

	close(toCreate)
	err = wg.Wait()

	return
}
