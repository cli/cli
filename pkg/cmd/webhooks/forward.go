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
	"os/signal"
	"syscall"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

const gitHubAPILocalURL = "http://api.github.localhost"
const gitHubAPIProdURL = "http://api.github.com"

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
		Use:   "forward --event=<event_type> --repo=<repo> --port=<port>",
		Short: "Receive test webhooks locally",
		Example: heredoc.Doc(`
			# create a dev webhook for the 'issue_open' event in the monalisa/smile repo and
			# forward payloads for the triggered event to localhost:9999

			$ gh webhooks forward --event=issue_open --repo=monalisa/smile --port=9999 
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
			} else {
				opts.Host = gitHubAPILocalURL
			}
			if opts.Port == 0 {
				fmt.Fprintf(opts.IO.Out, "No --port specified, printing webhook payloads to stdout \n")
			}

			wsURLString, err := createHook(&opts)
			if err != nil {
				return err
			}
			fmt.Printf("Received WS hook url from dotcom: %s \n", wsURLString)
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
			errorsChan := make(chan error)
			shutdown := make(chan os.Signal, 1)
			signal.Notify(shutdown, syscall.SIGTERM, syscall.SIGINT)
			defer signal.Stop(shutdown)

			// Retry connecting 3 times only
			for {
				if retries == 3 {
					fmt.Fprintf(opts.IO.Out, "Unable to connect to webhooks server, forwarding stopped.\n")
					return nil
				}

				conn, err := dial(token, wsURL, &opts)

				if err != nil {
					unwrapped := errors.Unwrap(err)
					var syscallErr *os.SyscallError
					// If the error is a TCP dial error, retry connecting
					if errors.As(unwrapped, &syscallErr) && (unwrapped.Error() == "connect: connection refused") {
						retries += 1
						time.Sleep(5 * time.Second)
						continue
					}
					return err
				}

				fmt.Fprintf(opts.IO.Out, "Connected to webhooks server, forwarding \n")
				retries = 0

				connCloser := &ConnCloser{Conn: conn}
				err = forwardEvents(connCloser, &opts, shutdown, events, errorsChan)

				// If an error happened and it's a server disconnect (1006), retry connecting
				if err != nil && websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
					retries += 1
					time.Sleep(10 * time.Second)
					continue
				}

				// If we get here, there was either no error or an error that's not a disconnect
				if err != nil {
					log.Printf("[LOG] some error happened: %v", err)
				}
				return nil
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.EventType, "event", "E", "", "Name of the event type to forward")
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Name of the repo where the webhook is installed")
	cmd.Flags().IntVarP(&opts.Port, "port", "P", 0, "Local port to receive webhooks on")
	return &cmd
}

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
	err = apiClient.REST(gitHubAPILocalURL, "POST", path, bytes.NewReader(reqBytes), &res)
	if err != nil {
		return "", err
	}
	return res.WsURL, nil
}

func dial(token string, url *url.URL, opts *hookOptions) (*websocket.Conn, error) {
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
	return c, nil
}

func forwardEvents(conn *ConnCloser, opts *hookOptions, shutdown chan os.Signal, events chan wsEventReceived, errors chan error) error {
	go func() {
		for {
			var ev wsEventReceived
			if !conn.IsClosed() {
				err := conn.ReadJSON(&ev)
				if err != nil {
					errors <- err
					continue
				}
			}

			events <- ev
		}
	}()

	for {
		select {
		case <-shutdown:
			log.Println("[LOG] received CTRL+C, closing connection")
			conn.Close()
			return nil
		case ev := <-events:
			log.Printf("[LOG] received event with headers: %v \n", ev.Header)
			req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d", opts.Port), bytes.NewReader(ev.Body))
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
			conn.WriteJSON(httpEventForward{
				Status: resp.StatusCode,
				Header: resp.Header,
				Body:   body,
			})
		case err := <-errors:
			return err
		}
	}

	return nil
}
