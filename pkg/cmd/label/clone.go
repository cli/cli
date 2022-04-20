package label

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
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
		Use:   "clone <source [HOST/]OWNER/REPO>",
		Short: "Clones labels from another repo",
		Long: heredoc.Doc(`
			Clones labels from another repo into the current repo on GitHub.

			All labels from the source repo will be copied to the current repo.
			Any labels in the current repo that are not in the source repo will
			not be deleted or modified.

			If any labels from the source repo already exist in the current repo
			the command will fail. You can overwrite existing labels in the
			current repo using the --force flag.

		`),
		Example: heredoc.Doc(`
			# clone and overwrite labels from the cli/cli repo into the current repo
			$ gh label clone cli/cli --force
		`),
		Args: cmdutil.ExactArgs(1, "cannot clone labels: source repo argument required"),
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

	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Overwrite labels in the current repo from the source repo.")

	return cmd
}

func cloneRun(opts *cloneOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	successCount, totalCount, err := cloneLabels(opts)
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		pluralize := func(num int) string {
			return utils.Pluralize(num, "label")
		}

		switch {
		case successCount == totalCount:
			fmt.Fprintf(opts.IO.Out, "%s Cloned %s from %s to %s\n", cs.SuccessIcon(), pluralize(successCount), opts.SourceRepo, baseRepo)
		case successCount != 0:
			fmt.Fprintf(opts.IO.Out, "%s Cloned %s of %d from %s to %s\n", cs.WarningIcon(), pluralize(successCount), totalCount, opts.SourceRepo, baseRepo)
		default:
			fmt.Fprintf(opts.IO.Out, "%s Failed to clone %s from %s to %s\n", cs.FailureIcon(), pluralize(totalCount), opts.SourceRepo, baseRepo)
		}
	}

	return nil
}

func cloneLabels(opts *cloneOptions) (successCount, totalCount int, err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return
	}

	opts.IO.StartProgressIndicator()
	defer opts.IO.StopProgressIndicator()

	successCount = 0
	labels, totalCount, err := listLabels(httpClient, opts.SourceRepo, listLimit)
	if err != nil {
		return
	}

	createOpts := createOptions{
		Force: opts.Force,
	}

	for _, label := range labels {
		createOpts.Name = label.Name
		createOpts.Description = label.Description
		createOpts.Color = label.Color

		createErr := createLabel(httpClient, baseRepo, &createOpts)
		if createErr != nil {
			if !errors.Is(createErr, errLabelAlreadyExists) {
				err = createErr
				return
			}
		} else {
			successCount++
		}
	}

	return
}
