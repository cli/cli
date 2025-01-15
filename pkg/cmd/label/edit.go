package label

import (
	"errors"
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
		Short: "Edit a label",
		Long: heredoc.Docf(`
			Update a label on GitHub.

			A label can be renamed using the %[1]s--name%[1]s flag.

			The label color needs to be 6 character hex value.
		`, "`"),
		Example: heredoc.Doc(`
			# update the color of the bug label
			$ gh label edit bug --color FF0000

			# rename and edit the description of the bug label
			$ gh label edit bug --name big-bug --description "Bigger than normal bug"
		`),
		Args: cmdutil.ExactArgs(1, "cannot update label: name argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.Name = args[0]
			opts.Color = strings.TrimPrefix(opts.Color, "#")
			if opts.Description == "" &&
				opts.Color == "" &&
				opts.NewName == "" {
				return cmdutil.FlagErrorf("specify at least one of `--color`, `--description`, or `--name`")
			}
			if runF != nil {
				return runF(&opts)
			}
			return editRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the label")
	cmd.Flags().StringVarP(&opts.Color, "color", "c", "", "Color of the label")
	cmd.Flags().StringVarP(&opts.NewName, "name", "n", "", "New name of the label")

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
		if errors.Is(err, errLabelAlreadyExists) {
			return fmt.Errorf("label with name %q already exists", opts.NewName)
		}
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		successMsg := fmt.Sprintf("%s Label %q updated in %s\n", cs.SuccessIcon(), opts.Name, ghrepo.FullName(baseRepo))
		fmt.Fprint(opts.IO.Out, successMsg)
	}

	return nil
}
