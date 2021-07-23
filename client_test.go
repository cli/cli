package liveshare

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}
	if client == nil {
		t.Error("client is nil")
	}
}

func TestNewClientValidConnection(t *testing.T) {
	connection := Connection{"1", "2", "3", "4"}

	client, err := NewClient(WithConnection(connection))
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}
	if client == nil {
		t.Error("client is nil")
	}
}

func TestNewClientWithInvalidConnection(t *testing.T) {
	connection := Connection{}

	if _, err := NewClient(WithConnection(connection)); err == nil {
		t.Error("err is nil")
	}
}

var upgrader = websocket.Upgrader{}

func newMockLiveShareServer() *httptest.Server {
	endpoint := func(w http.ResponseWriter, req *http.Request) {
		c, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer c.Close()

		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				fmt.Println(err)
				break
			}

			err = c.WriteMessage(mt, message)
			if err != nil {
				fmt.Println(err)
				break
			}

		}
	}

	return httptest.NewTLSServer(http.HandlerFunc(endpoint))
}

func TestClientJoin(t *testing.T) {
	// server := newMockLiveShareServer()
	// defer server.Close()

	// connection := Connection{
	// 	SessionID:     "session-id",
	// 	SessionToken:  "session-token",
	// 	RelaySAS:      "relay-sas",
	// 	RelayEndpoint: "sb" + strings.TrimPrefix(server.URL, "https"),
	// }

	// client, err := NewClient(WithConnection(connection))
	// if err != nil {
	// 	t.Errorf("error creating new client: %v", err)
	// }
	// ctx := context.Background()
	// if err := client.Join(ctx); err != nil {
	// 	t.Errorf("error joining client: %v", err)
	// }
}
