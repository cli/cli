package webhooks

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

const gitHubAPIProdURL = "http://api.github.com"

var retries = 0
var maxRetries = 3
var webhookRcvServerURL = ""

type hookOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	Host       string
	EventTypes []string
	Repo       string
	Port       int
}

func newCmdForward(f *cmdutil.Factory, runF func(*hookOptions) error) *cobra.Command {
	opts := hookOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}
	cmd := cobra.Command{
		Use:   "forward --events=<event_types> --repo=<repo> [--port=<port>] [--host=<host>]",
		Short: "Receive test events on a server running locally",
		Long: heredoc.Doc(`To output event payloads to stdout instead of sending to a server,
			omit the --port flag. If the --host flag is not specified, webhooks will be created against github.com`),
		Example: heredoc.Doc(`
			# create a dev webhook for the 'issue_open' event in the monalisa/smile repo in GitHub running locally, and
			# forward payloads for the triggered event to localhost:9999

			$ gh webhooks forward --events=issue_open --repo=monalisa/smile --port=9999 --host=api.github.localhost
		`),
		RunE: func(*cobra.Command, []string) error {
			if opts.EventTypes == nil {
				return cmdutil.FlagErrorf("`--events` flag required")
			}
			if opts.Repo == "" {
				return cmdutil.FlagErrorf("`--repo` flag required")
			}
			if opts.Host == "" {
				fmt.Fprintf(opts.IO.Out, "No --host specified, connecting to default GitHub.com \n")
				opts.Host = gitHubAPIProdURL
			}

			if opts.Port == 0 {
				fmt.Fprintf(opts.IO.Out, "No --port specified, printing webhook payloads to stdout \n")
			}

			if runF != nil {
				return runF(&opts)
			}

			wsURL, err := createHook(&opts)
			if err != nil {
				return err
			}

			cfg, err := opts.Config()
			if err != nil {
				return err
			}
			token, _ := cfg.AuthToken(opts.Host)
			if token == "" {
				return fmt.Errorf("you must be authenticated to run this command")
			}

			for {
				if retries >= maxRetries {
					return fmt.Errorf("unable to connect to webhooks server, forwarding stopped")
				}

				err := handleWebsocket(&opts, token, wsURL)
				if err != nil {
					// If the error is a server disconnect (1006), retry connecting
					if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
						retries++
						time.Sleep(5 * time.Second)
						continue
					}
					if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						return nil
					}
					return err
				}
			}
		},
	}
	cmd.Flags().StringSliceVarP(&opts.EventTypes, "events", "E", []string{}, "(required) Names of the event types to forward")
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "(required) Name of the repo where the webhook is installed")
	cmd.Flags().IntVarP(&opts.Port, "port", "P", 0, "(optional) Local port where the server which will receive webhooks is running")
	cmd.Flags().StringVarP(&opts.Host, "host", "H", "", "(optional) Host address of GitHub API, default: api.github.com")
	return &cmd
}

type wsEventReceived struct {
	Header http.Header
	Body   []byte
}

// handleWebsocket mediates between websocket server and local web server
func handleWebsocket(opts *hookOptions, token, url string) error {
	c, err := dial(token, url)
	if err != nil {
		return fmt.Errorf("error dialing to ws server: %w", err)
	}
	defer c.Close()
	retries = 0

	fmt.Fprintf(opts.IO.Out, "Forwarding Webhook events from GitHub...\n")
	for {
		var ev wsEventReceived
		err := c.ReadJSON(&ev)
		if err != nil {
			return fmt.Errorf("error receiving json event: %w", err)
		}

		resp, err := forwardEvent(opts, ev)
		if err != nil {
			fmt.Fprintf(opts.IO.Out, "Error forwarding event: %v\n", err)
			continue
		}

		err = c.WriteJSON(resp)
		if err != nil {
			return fmt.Errorf("error writing json event: %w", err)
		}
	}
}

// dial connects to the websocket server
func dial(token, url string) (*websocket.Conn, error) {
	h := make(http.Header)
	h.Set("Authorization", token)
	c, _, err := websocket.DefaultDialer.Dial(url, h)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type httpEventForward struct {
	Status int
	Header http.Header
	Body   []byte
}

// forwardEvent forwards events to the server running on the local port specified by the user
func forwardEvent(opts *hookOptions, ev wsEventReceived) (*httpEventForward, error) {
	event := ev.Header.Get("X-GitHub-Event")
	event = strings.ReplaceAll(event, "\n", "")
	event = strings.ReplaceAll(event, "\r", "")
	log.Printf("[LOG] received the following event: %v \n", event)
	if webhookRcvServerURL == "" {
		webhookRcvServerURL = fmt.Sprintf("http://localhost:%d", opts.Port)
	}

	req, err := http.NewRequest(http.MethodPost, webhookRcvServerURL, bytes.NewReader(ev.Body))
	if err != nil {
		return nil, err
	}

	for k := range ev.Header {
		req.Header.Set(k, ev.Header.Get(k))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &httpEventForward{
		Status: resp.StatusCode,
		Header: resp.Header,
		Body:   body,
	}, nil
}
