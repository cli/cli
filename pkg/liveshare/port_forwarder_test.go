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

	livesharetest "github.com/cli/cli/v2/pkg/liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

func TestNewPortForwarder(t *testing.T) {
	testServer, session, err := makeMockSession()
	if err != nil {
		t.Errorf("create mock client: %w", err)
	}
	defer testServer.Close()
	pf := NewPortForwarder(session, "ssh", 80, false)
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
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.startSharing", serverSharing),
		livesharetest.WithService("streamManager.getStream", getStream),
		livesharetest.WithStream("stream-id", stream),
	)
	if err != nil {
		t.Errorf("create mock session: %w", err)
	}
	defer testServer.Close()

	listen, err := net.Listen("tcp", ":8000")
	if err != nil {
		t.Fatal(err)
	}
	defer listen.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error)
	go func() {
		const name, remote = "ssh", 8000
		done <- NewPortForwarder(session, name, remote, false).ForwardToListener(ctx, listen)
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
			done <- fmt.Errorf("reading stream: %w", err)
		}
		if string(b) != "stream-data" {
			done <- fmt.Errorf("stream data is not expected value, got: %s", string(b))
		}
		if _, err := conn.Write([]byte("new-data")); err != nil {
			done <- fmt.Errorf("writing to stream: %w", err)
		}
		done <- nil
	}()

	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %w", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %w", err)
		}
	}
}

func TestPortForwarderTrafficMonitor(t *testing.T) {
	buf := bytes.NewBufferString("some-input")
	session := &Session{keepAliveReason: make(chan string, 1)}
	trafficType := "io"

	tm := newTrafficMonitor(buf, session, trafficType)
	l := len(buf.Bytes())

	bb := make([]byte, l)
	n, err := tm.Read(bb)
	if err != nil {
		t.Errorf("failed to read from traffic monitor: %w", err)
	}
	if n != l {
		t.Errorf("expected to read %d bytes, got %d", l, n)
	}

	keepAliveReason := <-session.keepAliveReason
	if keepAliveReason != trafficType {
		t.Errorf("expected keep alive reason to be %s, got %s", trafficType, keepAliveReason)
	}
}
