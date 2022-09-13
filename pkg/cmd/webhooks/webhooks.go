package webhooks

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type hookOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	EventType string
	Repo      string
	Port      int
}

func NewCmdWebhooks(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks <command>",
		Short: "Test a webhooks integration",
		Long:  `Create a test webhook, trigger and receive it locally`,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdForward(f, nil))

	return cmd
}

func newCmdForward(f *cmdutil.Factory, runF func(*hookOptions) error) *cobra.Command {
	opts := hookOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}
	cmd := cobra.Command{
		Use:   "forward --event=<event_type> --repo=<repo> --port=<port>",
		Short: "Receive test webhooks locally",
		Example: heredoc.Doc(`
			# create a dev webhook for the 'issue_open' event in the monalisa/smile repo and
			# forward payloads for the triggered event to localhost:9999

			$ gh webhooks forward --event=issue_open --repo=monalisa/smile --port=9999 
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Received gh webhooks forward command with event flag: %#+v, repo: %#+v, port: %#+v", opts.EventType, opts.Repo, opts.Port)
			wsURLString, err := createHook(&opts)
			if err != nil {
				return err
			}
			wsURL, err := url.Parse(wsURLString)
			if err != nil {
				return err
			}
			err = forwardEvents(wsURL)
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.EventType, "event", "E", "", "Name of the event type to forward")
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Name of the repo where the webhook is installed")
	cmd.Flags().IntVarP(&opts.Port, "port", "P", 9999, "Local port to receive webhooks on")
	return &cmd
}

func createHook(o *hookOptions) (string, error) {
	// post to /repositories/:repository_id/hooks, operation_id: "repos/create-webhook"
	return "", nil
}

func forwardEvents(u *url.URL) error {
	// open ws connection and read
	return nil
}
