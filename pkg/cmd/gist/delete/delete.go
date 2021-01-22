package delete

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)

	Selector string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "delete {<gist ID> | <gist URL>}",
		Short: "Delete a gist",
		Args:  cmdutil.MinimumArgs(1, "cannot delete: gist argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]
			if runF != nil {
				return runF(&opts)
			}
			return deleteRun(&opts)
		},
	}
	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	gistID := opts.Selector

	if strings.Contains(gistID, "/") {
		id, err := shared.GistIDFromURL(gistID)
		if err != nil {
			return err
		}
		gistID = id
	}
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(client)

	gist, err := shared.GetGist(client, ghinstance.OverridableDefault(), gistID)
	if err != nil {
		return err
	}
	username, err := api.CurrentLoginName(apiClient, ghinstance.OverridableDefault())
	if err != nil {
		return err
	}

	if username != gist.Owner.Login {
		return fmt.Errorf("You do not own this gist.")
	}

	err = deleteGist(apiClient, ghinstance.OverridableDefault(), gistID)
	if err != nil {
		return err
	}

	return nil
}

func deleteGist(apiClient *api.Client, hostname string, gistID string) error {
	path := "gists/" + gistID
	err := apiClient.REST(hostname, "DELETE", path, nil, nil)
	if err != nil {
		return err
	}
	return nil
}
