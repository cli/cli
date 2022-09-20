package webhooks

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdForward(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	f := &cmdutil.Factory{
		IOStreams: ios,
	}

	var opts *hookOptions
	cmd := newCmdForward(f, func(o *hookOptions) error {
		opts = o
		return nil
	})

	event := "issues"
	port := 9999
	repo := "monalisa/smile"
	host := "api.github.localhost"
	run := fmt.Sprintf("--event %s --port %d --repo %s --host %s", event, port, repo, host)

	argv, err := shlex.Split(run)
	assert.NoError(t, err)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err = cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Equal(t, event, opts.EventType)
	assert.Equal(t, port, opts.Port)
	assert.Equal(t, repo, opts.Repo)
	assert.Equal(t, host, opts.Host)
}

func TestForwardRun(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		HttpClient: func() (*http.Client, error) {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			return &http.Client{Transport: tr}, nil
		},
	}

	wsServer := getWSServer()
	defer wsServer.Close()

	ghAPIServer := getGHAPIServer(wsServer.URL)
	defer ghAPIServer.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	webhookRcvServer, forwarded := getWebhookRcvServer(wg.Done)
	defer webhookRcvServer.Close()
	webhookRcvServerURL = webhookRcvServer.URL

	maxRetries = 1
	cmd := newCmdForward(f, nil)
	event := "issues"
	splitURL := strings.Split(webhookRcvServerURL, ":")
	port := splitURL[len(splitURL)-1]
	repo := "monalisa/smile"
	host := strings.TrimPrefix(ghAPIServer.URL, "https://")

	run := fmt.Sprintf("--event %s --port %s --repo %s --host %s", event, port, repo, host)
	argv, err := shlex.Split(run)
	assert.NoError(t, err)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err = cmd.ExecuteC()
	assert.NoError(t, err)
	wg.Wait()
	assert.Equal(t, "lol\n", string(forwarded.event.Body))
	assert.Equal(t, forwarded.event.Header.Get("Someheader"), "somevalue")
}

type forwarded struct {
	event localEvent
	done  func()
}

type localEvent struct {
	Body   []byte `json:"body"`
	Header http.Header
}

func (w *forwarded) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var event localEvent
	err := json.NewDecoder(req.Body).Decode(&event)
	if err != nil {
		fmt.Errorf("failed to decode request: %s\n", err)
	}
	event.Header = http.Header{}
	for h := range req.Header {
		event.Header.Add(h, req.Header.Get(h))
	}
	w.event = event
	_, err = res.Write([]byte("OK"))
	if err != nil {
		fmt.Errorf("failed to write response: %s\n", err)
	}

	w.done()
}

func getWebhookRcvServer(done func()) (*httptest.Server, *forwarded) {
	s := &forwarded{done: done}
	return httptest.NewServer(s), s
}

func getGHAPIServer(wsServerURL string) *httptest.Server {
	ghAPIHandler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		wsURL := strings.Replace(wsServerURL, "http", "ws", 1)
		ret := createHookResponse{
			WsURL: wsURL,
		}
		err := json.NewEncoder(res).Encode(ret)
		if err != nil {
			fmt.Errorf("failed to write response: %s\n", err)
		}
	})
	return httptest.NewTLSServer(ghAPIHandler)
}

func getWSServer() *httptest.Server {
	wsHandler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		var upgrader = websocket.Upgrader{}
		c, err := upgrader.Upgrade(res, req, nil)
		if err != nil {
			fmt.Errorf("failed to upgrade: %s\n", err)
		}
		defer c.Close()

		header := http.Header{}
		header.Add("Someheader", "somevalue")
		msg := wsRequest{
			Header: header,
			Body:   []byte(`{"body": "bG9sCg=="}`),
		}
		send, _ := json.Marshal(msg)
		err = c.WriteMessage(1, send)
		if err != nil {
			fmt.Errorf("failed to write: %s\n", err)
		}
		err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1000, "woops"))
		if err != nil {
			fmt.Errorf("failed to write: %s\n", err)
		}
	})
	return httptest.NewServer(wsHandler)
}

type wsRequest struct {
	Header http.Header
	Body   []byte
}
