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

var maxRetries = 3
var webhookRcvServerURL = ""

type hookOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)

	Host      string
	EventType string
	Repo      string
	Port      int
}

func newCmdForward(f *cmdutil.Factory, runF func(*hookOptions) error) *cobra.Command {
	opts := hookOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Config:     f.Config,
	}
	cmd := cobra.Command{
		Use:   "forward --event=<event_type> --repo=<repo> --port=<port> [--host=<host>]",
		Short: "Receive test webhooks locally",
		Example: heredoc.Doc(`
			# create a dev webhook for the 'issue_open' event in the monalisa/smile repo in GitHub running locally, and
			# forward payloads for the triggered event to localhost:9999

			$ gh webhooks forward --event=issue_open --repo=monalisa/smile --port=9999 --host=api.github.localhost
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.EventType == "" {
				return cmdutil.FlagErrorf("`--event` flag required")
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

			var retries int
			events := make(chan wsEventReceived)
			replies := make(chan httpEventForward)

			go forwardEvents(&opts, events, replies)

			for {
				if retries >= maxRetries {
					fmt.Fprintf(opts.IO.Out, "Unable to connect to webhooks server, forwarding stopped.\n")
					return nil
				}

				done := make(chan struct{})
				defer close(done)
				conn, err := handleWebsocket(token, wsURL, &opts, events)

				if err != nil {
					unwrapped := errors.Unwrap(err)
					var syscallErr *os.SyscallError
					// If the error is a TCP handleWebsocket error, retry connecting
					if errors.As(unwrapped, &syscallErr) && (unwrapped.Error() == "connect: connection refused") {
						retries += 1
						time.Sleep(5 * time.Second)
						continue
					}
					// If the error is a server disconnect (1006), retry connecting
					if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
						retries += 1
						time.Sleep(10 * time.Second)
						continue
					}

					if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						return nil
					}
					return err
				}

				fmt.Fprintf(opts.IO.Out, "Connected to webhooks server, forwarding \n")
				retries = 0

				go func() {
					for {
						select {
						case <-done:
							return
						case reply := <-replies:
							err := conn.WriteJSON(reply)
							if err != nil {
								return
							}
						}
					}
				}()
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.EventType, "event", "E", "", "Name of the event type to forward")
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Name of the repo where the webhook is installed")
	cmd.Flags().IntVarP(&opts.Port, "port", "P", 0, "Local port to receive webhooks on")
	cmd.Flags().StringVarP(&opts.Host, "host", "H", "", "Host address of GitHub API")
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
		Events: []string{o.EventType},
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

// handleWebsocket connects to the websocket server and saves received events to a channel
func handleWebsocket(token string, url *url.URL, opts *hookOptions, events chan wsEventReceived) (*websocket.Conn, error) {
	fmt.Fprintf(opts.IO.Out, "Attempting to connect to webhooks server...\n")
	h := make(http.Header)
	h.Set("Authorization", token)

	c, resp, err := websocket.DefaultDialer.Dial(url.String(), h)
	if err != nil {
		if resp != nil {
			bts, _ := io.ReadAll(resp.Body)
			err = fmt.Errorf("ws err %d - %s - %v", resp.StatusCode, bts, err)
		}
		return nil, err
	}

	defer c.Close()

	for {
		var ev wsEventReceived
		err := c.ReadJSON(&ev)
		if err != nil {
			return nil, err
		}
		events <- ev
	}

	return c, nil
}

// forwardEvents forwards events to the server running on the local port specified by the user
func forwardEvents(opts *hookOptions, events chan wsEventReceived, replies chan httpEventForward) error {
	for ev := range events {
		// TODO remove before merging
		log.Printf("[LOG] received event with headers: %v \n", ev.Header)
		if webhookRcvServerURL == "" {
			webhookRcvServerURL = fmt.Sprintf("http://localhost:%d", opts.Port)
		}

		req, err := http.NewRequest(http.MethodPost, webhookRcvServerURL, bytes.NewReader(ev.Body))
		if err != nil {
			return err
		}

		for k := range ev.Header {
			req.Header.Set(k, ev.Header.Get(k))
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		replies <- httpEventForward{
			Status: resp.StatusCode,
			Header: resp.Header,
			Body:   body,
		}
	}

	return nil
}
