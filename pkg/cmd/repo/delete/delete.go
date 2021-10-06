package delete

import (
	"net/http"

	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	RepoArg    string
	Confirmed  bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "delete <repository>",
		Short: "Delete a repository",
		Long: `Delete a GitHub repository.

		Ensure that you have authorized the \"delete_repo\" scope:  gh auth refresh -h github.com -s delete_repo"`,
		Args: cmdutil.ExactArgs(1, "cannot delete: repository argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.RepoArg = args[0]
			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "Confirm deletion without prompting")
	return cmd
}

func deleteRun(opts *DeleteOptions) error {

	return nil
}
