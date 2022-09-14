package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
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

const gitHubAPIBaseURL = "http://api.github.localhost"

type hookOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

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
	ContentType string `json:"content-type"`
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
			fmt.Printf("Received gh webhooks forward command with event flag: %v, repo: %v, port: %v \n", opts.EventType, opts.Repo, opts.Port)

			wsURLString, err := createHook(&opts)
			if err != nil {
				return err
			}
			fmt.Printf("Received hook url from dotcom: %s \n", wsURLString)
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
	err = apiClient.REST(gitHubAPIBaseURL, "POST", path, bytes.NewReader(reqBytes), &res)
	if err != nil {
		return "", err
	}
	return res.WsURL, nil
}

func forwardEvents(u *url.URL) error {
	// handle signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(shutdown)

	// dial ws server
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
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
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("error reading message:", err)
				return err
			}
			log.Printf("received message: %s", message)
		}
	}

	return nil
}
