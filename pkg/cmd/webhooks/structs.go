package webhooks

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

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

type wsEventReceived struct {
	Header http.Header
	Body   []byte
}

type httpEventForward struct {
	Status int
	Header http.Header
	Body   []byte
}

type ConnCloser struct {
	*websocket.Conn
	mu     sync.Mutex
	closed bool
}

func (c *ConnCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.Conn.Close()
}

func (c *ConnCloser) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}
