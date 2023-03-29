package delete

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)

	Selector string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := DeleteOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "delete {<id> | <url>}",
		Short: "Delete a gist",
		Args:  cmdutil.ExactArgs(1, "cannot delete: gist argument required"),
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

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	apiClient := api.NewClientFromHTTP(client)
	if err := deleteGist(apiClient, host, gistID); err != nil {
		if errors.Is(err, shared.NotFoundErr) {
			return fmt.Errorf("unable to delete gist %s: either the gist is not found or it is not owned by you", gistID)
		}
		return err
	}

	return nil
}

func deleteGist(apiClient *api.Client, hostname string, gistID string) error {
	path := "gists/" + gistID
	err := apiClient.REST(hostname, "DELETE", path, nil, nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return shared.NotFoundErr
		}
		return err
	}
	return nil
}
