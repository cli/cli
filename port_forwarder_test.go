package liveshare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	livesharetest "github.com/github/go-liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

func TestNewPortForwarder(t *testing.T) {
	testServer, client, err := makeMockJoinedClient()
	if err != nil {
		t.Errorf("create mock client: %v", err)
	}
	defer testServer.Close()
	server, err := NewServer(client)
	if err != nil {
		t.Errorf("create new server: %v", err)
	}
	pf := NewPortForwarder(client, server, 80)
	if pf == nil {
		t.Error("port forwarder is nil")
	}
}

func TestPortForwarderStart(t *testing.T) {
	streamName, streamCondition := "stream-name", "stream-condition"
	serverSharing := func(req *jsonrpc2.Request) (interface{}, error) {
		return Port{StreamName: streamName, StreamCondition: streamCondition}, nil
	}
	getStream := func(req *jsonrpc2.Request) (interface{}, error) {
		return "stream-id", nil
	}

	stream := bytes.NewBufferString("stream-data")
	testServer, client, err := makeMockJoinedClient(
		livesharetest.WithService("serverSharing.startSharing", serverSharing),
		livesharetest.WithService("streamManager.getStream", getStream),
		livesharetest.WithStream("stream-id", stream),
	)
	if err != nil {
		t.Errorf("create mock client: %v", err)
	}
	defer testServer.Close()

	server, err := NewServer(client)
	if err != nil {
		t.Errorf("create new server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pf := NewPortForwarder(client, server, 8000)
	done := make(chan error)

	go func() {
		if err := server.StartSharing(ctx, "http", 8000); err != nil {
			done <- fmt.Errorf("start sharing: %v", err)
		}
		done <- pf.Forward(ctx)
	}()

	go func() {
		var conn net.Conn
		retries := 0
		for conn == nil && retries < 2 {
			conn, err = net.DialTimeout("tcp", ":8000", 2*time.Second)
			time.Sleep(1 * time.Second)
		}
		if conn == nil {
			done <- errors.New("failed to connect to forwarded port")
		}
		b := make([]byte, len("stream-data"))
		if _, err := conn.Read(b); err != nil && err != io.EOF {
			done <- fmt.Errorf("reading stream: %v", err)
		}
		if string(b) != "stream-data" {
			done <- fmt.Errorf("stream data is not expected value, got: %v", string(b))
		}
		if _, err := conn.Write([]byte("new-data")); err != nil {
			done <- fmt.Errorf("writing to stream: %v", err)
		}
		done <- nil
	}()

	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %v", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %v", err)
		}
	}
}
