package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"
)

// A PortForwarder forwards TCP traffic between a port on a remote
// LiveShare host and a local port.
type PortForwarder struct {
	client *Client
	server *Server
	port   int
}

// NewPortForwarder creates a new PortForwarder that connects a given client, server and port.
func NewPortForwarder(client *Client, server *Server, port int) *PortForwarder {
	return &PortForwarder{
		client: client,
		server: server,
		port:   port,
	}
}

// Forward enables port forwarding. It accepts and handles TCP
// connections until it encounters the first error, which may include
// context cancellation. Its result is non-nil.
func (l *PortForwarder) Forward(ctx context.Context) (err error) {
	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", l.port))
	if err != nil {
		return fmt.Errorf("error listening on TCP port: %v", err)
	}
	defer safeClose(listen, &err)

	errc := make(chan error, 1)
	sendError := func(err error) {
		// Use non-blocking send, to avoid goroutines getting
		// stuck in case of concurrent or sequential errors.
		select {
		case errc <- err:
		default:
		}
	}
	go func() {
		for {
			conn, err := listen.Accept()
			if err != nil {
				sendError(err)
				return
			}

			go func() {
				if err := l.handleConnection(ctx, conn); err != nil {
					sendError(err)
				}
			}()
		}
	}()

	return awaitError(ctx, errc)
}

// ForwardWithConn handles port forwarding for a single connection.
func (l *PortForwarder) ForwardWithConn(ctx context.Context, conn io.ReadWriteCloser) error {
	// Create buffered channel so that send doesn't get stuck after context cancellation.
	errc := make(chan error, 1)
	go func() {
		if err := l.handleConnection(ctx, conn); err != nil {
			errc <- err
		}
	}()
	return awaitError(ctx, errc)
}

func awaitError(ctx context.Context, errc <-chan error) error {
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err() // canceled
	}
}

// handleConnection handles forwarding for a single accepted connection, then closes it.
func (l *PortForwarder) handleConnection(ctx context.Context, conn io.ReadWriteCloser) (err error) {
	defer safeClose(conn, &err)

	channel, err := l.client.openStreamingChannel(ctx, l.server.streamName, l.server.streamCondition)
	if err != nil {
		return fmt.Errorf("error opening streaming channel for new connection: %v", err)
	}
	defer safeClose(channel, &err)

	errs := make(chan error, 2)
	copyConn := func(w io.Writer, r io.Reader) {
		_, err := io.Copy(w, r)
		errs <- err
	}
	go copyConn(conn, channel)
	go copyConn(channel, conn)

	// await result
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil && err != io.EOF {
			return fmt.Errorf("tunnel connection: %v", err)
		}
	}
	return nil
}

// safeClose reports the error (to *err) from closing the stream only
// if no other error was previously reported.
func safeClose(closer io.Closer, err *error) {
	closeErr := closer.Close()
	if *err == nil {
		*err = closeErr
	}
}
