package webhooks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

type wsMsg struct {
	Header http.Header
	Body   []byte
}

type localEvent struct {
	Body   string `json:"body"`
	Header http.Header
}

// forwarded adheres to the http.Handler interface so we can record incoming requests
type forwarded struct {
	event            localEvent
	t                *testing.T
	done             func()
	returnErr        bool
	returnStatusCode int
}

func (f *forwarded) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	f.t.Helper()
	var event localEvent
	err := json.NewDecoder(req.Body).Decode(&event)
	if err != nil {
		f.t.Errorf("failed to decode request: %s\n", err)
		return
	}
	event.Header = http.Header{}
	for h := range req.Header {
		event.Header.Add(h, req.Header.Get(h))
	}
	f.event = event

	if f.returnErr {
		res.WriteHeader(f.returnStatusCode)
		return
	}
	_, err = res.Write([]byte("OK"))
	if err != nil {
		f.t.Errorf("failed to write response: %s\n", err)
		return
	}

	f.done()
}

// Creates a local HTTP server to receive test events
func getWebhookRcvServer(t *testing.T, done func(), returnErr bool, returnStatusCode int) (*httptest.Server, *forwarded) {
	s := &forwarded{
		t:                t,
		done:             done,
		returnErr:        returnErr,
		returnStatusCode: returnStatusCode,
	}
	return httptest.NewServer(s), s
}

// Creates a mock GitHub API server
func getGHAPIServer(wsServerURL string, wantErr bool, wantStatusCode int, t *testing.T) *httptest.Server {
	t.Helper()
	ghAPIHandler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if wantErr {
			res.WriteHeader(wantStatusCode)
			return
		}
		wsURL := strings.Replace(wsServerURL, "http", "ws", 1)
		ret := createHookResponse{
			WsURL: wsURL,
		}
		err := json.NewEncoder(res).Encode(ret)
		if err != nil {
			t.Errorf("failed to write response: %s\n", err)
			return
		}
	})
	return httptest.NewTLSServer(ghAPIHandler)
}

// Creates a mock websocket server that forwards test events to the CLI
func getWSServer(t *testing.T, returnErr bool) *httptest.Server {
	t.Helper()
	wsHandler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		var upgrader = websocket.Upgrader{}
		c, err := upgrader.Upgrade(res, req, nil)
		if err != nil {
			t.Errorf("failed to upgrade: %s\n", err)
			return
		}
		defer c.Close()

		if returnErr {
			err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseMessageTooBig, ""))
			if err != nil {
				t.Errorf("failed to write: %s\n", err)
			}
			return
		}

		header := http.Header{}
		header.Add("Someheader", "somevalue")
		msg := wsMsg{
			Header: header,
			Body:   []byte(`{"body": "lol"}`),
		}
		send, _ := json.Marshal(msg)
		err = c.WriteMessage(1, send)
		if err != nil {
			t.Errorf("failed to write: %s\n", err)
			return
		}
		err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			t.Errorf("failed to write: %s\n", err)
			return
		}
	})
	return httptest.NewServer(wsHandler)
}
