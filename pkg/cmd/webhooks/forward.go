/* Package webhooks CLI commands

TODO:
1. Get the auth header from somewhere inside this codebase.
2. Handle process signal cancellation.
3. TODO below (closing the body out of the loop)
7. Reconnect when disconnected abruptly from the server.

Done:
6. Get the right GitHub Host (hopefully from shared code): maybe default to .com and have an optional -h for github.localhost.
5. What happens when we don't pass events: make it required.
4. Add a --print (or if you don't pass a port).

*/

package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

const gitHubAPILocalURL = "http://api.github.localhost"
const gitHubAPIProdURL = "http://api.github.com"

type hookOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Host      string
	EventType string
	Repo      string
	Port      int
}

type createHookRequest struct {
	Name   string     `json:"name"`
	Events []string   `json:"events"`
	Active bool       `json:"active"`
	Config hookConfig `json:"config"`
}

type hookConfig struct {
	ContentType string `json:"content_type"`
	InsecureSSL string `json:"insecure_ssl"`
	URL         string `json:"url"`
}

type createHookResponse struct {
	Active bool       `json:"active"`
	Config hookConfig `json:"config"`
	Events []string   `json:"events"`
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	URL    string     `json:"url"`
	WsURL  string     `json:"ws_url"`
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
			fmt.Printf("Received gh webhooks forward command with event flag: %v, repo: %v, port: %v \n", opts.EventType, opts.Repo, opts.Port)
			//
			// wsURLString, err := createHook(&opts)
			// if err != nil {
			// 	return err
			// }
			// fmt.Printf("Received hook url from dotcom: %s \n", wsURLString)
			// wsURL, err := url.Parse(wsURLString)
			// if err != nil {
			// 	return err
			// }

			wsURL := url.URL{Scheme: "ws", Host: "localhost:8088", Path: "/hi"}

			err := forwardEvents(&wsURL, "TODO!", opts.Port)
			if err != nil {
				return err
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
		Name:   "dev",
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

type wsRequest struct {
	Header http.Header
	Body   []byte
}

type wsResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

func forwardEvents(u *url.URL, token string, port int) error {
	// handle signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(shutdown)

	// dial ws server
	h := make(http.Header)
	h.Set("Authorization", token)

	c, resp, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		if resp != nil {
			bts, _ := io.ReadAll(resp.Body)
			err = fmt.Errorf("ws err %d - %s - %v", resp.StatusCode, bts, err)
		}
		return err
	}
	defer c.Close()

	// read messages
	for {
		select {
		case <-shutdown:
			log.Println("received CTRL+C, closing connection")
			return nil
		default:
			var r wsRequest
			err := c.ReadJSON(&r)
			if err != nil {
				log.Println("error reading message:", err)
				return err
			}

			fmt.Printf("received message: %#+v \n", r)

			// req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%d", port), bytes.NewReader(r.Body))
			// if err != nil {
			// 	return err
			// }
			//
			// for k := range r.Header {
			// 	req.Header.Set(k, r.Header.Get(k))
			// }
			// resp, err := http.DefaultClient.Do(req)
			// if err != nil {
			// 	return err
			// }
			// defer resp.Body.Close() // TODO: This is inside a loop!
			// body, err := io.ReadAll(resp.Body)
			// if err != nil {
			// 	return err
			// }
			// c.WriteJSON(wsResponse{
			// 	Status: resp.StatusCode,
			// 	Header: resp.Header,
			// 	Body:   body,
			// })
		}
	}

	return nil
}
