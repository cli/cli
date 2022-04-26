package label

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type editOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Color       string
	Description string
	Name        string
	NewName     string
}

func newCmdEdit(f *cmdutil.Factory, runF func(*editOptions) error) *cobra.Command {
	opts := editOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Updates an existing label",
		Long: heredoc.Doc(`
			Updates an existing label on GitHub. You can also rename a label.

			Must specify name for the label. The description or color is updated
			to either of the values specified.

			Color needs to be 6 character hex value.
		`),
		Example: heredoc.Doc(`
			# update just the color for the bug label
			$ gh label edit bug --color FF0000

			# rename a label with a new description
			$ gh label edit bug --rename issue --description "Bug or design issue"
	  `),
		Args: cmdutil.ExactArgs(1, "cannot update label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]
			opts.Color = strings.TrimPrefix(opts.Color, "#")
			if runF != nil {
				return runF(&opts)
			}
			return editRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the label.")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Color of the label. If not specified one will be selected at random.")
	cmd.Flags().StringVarP(&opts.NewName, "rename", "", "", "Rename an existing label to the given name.")

	return cmd
}

func editRun(opts *editOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	err = updateLabel(apiClient, baseRepo, opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q updated in %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}
