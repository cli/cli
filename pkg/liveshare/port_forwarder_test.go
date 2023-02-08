package liveshare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	livesharetest "github.com/cli/cli/v2/pkg/liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

func TestNewPortForwarder(t *testing.T) {
	testServer, session, err := makeMockSession()
	if err != nil {
		t.Errorf("create mock client: %v", err)
	}
	defer testServer.Close()
	pf := NewPortForwarder(session, "ssh", 80, false)
	if pf == nil {
		t.Error("port forwarder is nil")
	}
}

type portUpdateNotification struct {
	PortNotification
	conn *jsonrpc2.Conn
}

func TestPortForwarderStart(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("fails intermittently in CI: https://github.com/cli/cli/issues/5338")
	}

	streamName, streamCondition := "stream-name", "stream-condition"
	const port = 8000
	sendNotification := make(chan portUpdateNotification)
	serverSharing := func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		// Send the PortNotification that will be awaited on in session.StartSharing
		sendNotification <- portUpdateNotification{
			PortNotification: PortNotification{
				Port:       port,
				ChangeKind: PortChangeKindStart,
			},
			conn: conn,
		}
		return Port{StreamName: streamName, StreamCondition: streamCondition}, nil
	}
	getStream := func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		return "stream-id", nil
	}

	stream := bytes.NewBufferString("stream-data")
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.startSharing", serverSharing),
		livesharetest.WithService("streamManager.getStream", getStream),
		livesharetest.WithStream("stream-id", stream),
	)
	if err != nil {
		t.Errorf("create mock session: %v", err)
	}
	defer testServer.Close()

	listen, err := net.Listen("tcp", "127.0.0.1:8000")
	if err != nil {
		t.Fatal(err)
	}
	defer listen.Close()
	tcpListener, ok := listen.(*net.TCPListener)
	if !ok {
		t.Fatal("net.Listen did not return a TCPListener")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		notif := <-sendNotification
		_, _ = notif.conn.DispatchCall(context.Background(), "serverSharing.sharingSucceeded", notif)
	}()

	done := make(chan error, 2)
	go func() {
		done <- NewPortForwarder(session, "ssh", port, false).ForwardToListener(ctx, tcpListener)
	}()

	go func() {
		var conn net.Conn

		// We retry DialTimeout in a loop to deal with a race in PortForwarder startup.
		for tries := 0; conn == nil && tries < 2; tries++ {
			conn, err = net.DialTimeout("tcp", ":8000", 2*time.Second)
			if conn == nil {
				time.Sleep(1 * time.Second)
			}
		}
		if conn == nil {
			done <- errors.New("failed to connect to forwarded port")
			return
		}
		b := make([]byte, len("stream-data"))
		if _, err := conn.Read(b); err != nil && err != io.EOF {
			done <- fmt.Errorf("reading stream: %w", err)
			return
		}
		if string(b) != "stream-data" {
			done <- fmt.Errorf("stream data is not expected value, got: %s", string(b))
			return
		}
		if _, err := conn.Write([]byte("new-data")); err != nil {
			done <- fmt.Errorf("writing to stream: %w", err)
			return
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

func TestPortForwarderTrafficMonitor(t *testing.T) {
	buf := bytes.NewBufferString("some-input")
	session := &Session{keepAliveReason: make(chan string, 1)}
	trafficType := "io"

	tm := newTrafficMonitor(buf, session, trafficType)
	l := len(buf.Bytes())

	bb := make([]byte, l)
	n, err := tm.Read(bb)
	if err != nil {
		t.Errorf("failed to read from traffic monitor: %v", err)
	}
	if n != l {
		t.Errorf("expected to read %d bytes, got %d", l, n)
	}

	keepAliveReason := <-session.keepAliveReason
	if keepAliveReason != trafficType {
		t.Errorf("expected keep alive reason to be %s, got %s", trafficType, keepAliveReason)
	}
}
