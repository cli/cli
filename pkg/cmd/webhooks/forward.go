package webhooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/api"
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
		Use:   "forward --events=<event_types> --repo=<repo> --port=<port> [--host=<host>]",
		Short: "Receive test webhooks locally",
		Example: heredoc.Doc(`
			# create a dev webhook for the 'issue_open' event in the monalisa/smile repo in GitHub running locally, and
			# forward payloads for the triggered event to localhost:9999

			$ gh webhooks forward --events=issue_open --repo=monalisa/smile --port=9999 --host=api.github.localhost
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			wsURLString, err := createHook(&opts)
			if err != nil {
				return err
			}

			wsURL, err := url.Parse(wsURLString)
			if err != nil {
				return err
			}

			cfg, err := opts.Config()
			if err != nil {
				return err
			}
			token, _ := cfg.AuthToken(opts.Host)

			for {
				if retries >= maxRetries {
					fmt.Fprintf(opts.IO.Out, "Unable to connect to webhooks server, forwarding stopped.\n")
					return nil
				}

				err := handleWebsocket(&opts, token, wsURL)
				if err != nil {
					unwrapped := errors.Unwrap(err)
					var syscallErr *os.SyscallError
					// If the error is a TCP handleWebsocket error or a server disconnect (1006), retry connecting
					if errors.As(unwrapped, &syscallErr) && (unwrapped.Error() == "connect: connection refused") || websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
						retries += 1
						time.Sleep(5 * time.Second)
						continue
					}
					if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						return nil
					}
				}
			}
		},
	}
	cmd.Flags().StringSliceVarP(&opts.EventTypes, "events", "E", []string{}, "Name of the event types to forward")
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Name of the repo where the webhook is installed")
	cmd.Flags().IntVarP(&opts.Port, "port", "P", 0, "Local port to receive webhooks on")
	cmd.Flags().StringVarP(&opts.Host, "host", "H", "", "Host address of GitHub API (default: api.github.com)")
	return &cmd
}

// createHook issues a request against the GitHub API to create a dev webhook
func createHook(o *hookOptions) (string, error) {
	httpClient, err := o.HttpClient()
	if err != nil {
		return "", err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	path := fmt.Sprintf("repos/%s/hooks", o.Repo)
	req := createHookRequest{
		Name:   "cli",
		Events: o.EventTypes,
		Active: true,
		Config: hookConfig{
			ContentType: "json",
			InsecureSSL: "0",
		},
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	var res createHookResponse
	err = apiClient.REST(o.Host, "POST", path, bytes.NewReader(reqBytes), &res)
	if err != nil {
		return "", err
	}
	return res.WsURL, nil
}

// handleWebsocket mediates between websocket server and local web server
func handleWebsocket(opts *hookOptions, token string, url *url.URL) error {
	fmt.Fprintf(opts.IO.Out, "Attempting to connect to webhooks server...\n")
	c, err := dial(token, url)
	if err != nil {
		return err
	}
	defer c.Close()
	retries = 0

	for {
		var ev wsEventReceived
		err := c.ReadJSON(&ev)
		if err != nil {
			return err
		}

		resp, err := forwardEvent(opts, ev)
		if err != nil {
			continue
		}

		err = c.WriteJSON(resp)
		if err != nil {
			return err
		}
	}
}

// dial connects to the websocket server
func dial(token string, url *url.URL) (*websocket.Conn, error) {
	h := make(http.Header)
	h.Set("Authorization", token)
	c, _, err := websocket.DefaultDialer.Dial(url.String(), h)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// forwardEvent forwards events to the server running on the local port specified by the user
func forwardEvent(opts *hookOptions, ev wsEventReceived) (*httpEventForward, error) {
	log.Printf("[LOG] received event with headers: %v \n", ev.Header)
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
