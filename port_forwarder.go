package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"
)

// A PortForwarder forwards TCP traffic between a local TCP port and a LiveShare session.
type PortForwarder struct {
	session               *Session
	name                  string
	localPort, remotePort int
}

// NewPortForwarder creates a new PortForwarder that forwards traffic
// between the local port and the container's remote port over the
// specified Live Share session. The name describes the purpose of the
// remote port or service.
//
// TODO(adonovan): the localPort param is redundant wrt ForwardWithConn.
// Simpler: do away with the NewPortForwarder type altogether:
//
// - ForwardToLocalPort(ctx, session, name, remote, local)
// - ForwardToConnection(ctx, session, name, remote, conn)
func NewPortForwarder(session *Session, name string, localPort, remotePort int) *PortForwarder {
	return &PortForwarder{
		session:    session,
		name:       name,
		localPort:  localPort,
		remotePort: remotePort,
	}
}

// Forward enables port forwarding. It accepts and handles TCP
// connections until it encounters the first error, which may include
// context cancellation. Its result is non-nil.
func (fwd *PortForwarder) Forward(ctx context.Context) (err error) {
	id, err := fwd.shareRemotePort(ctx)
	if err != nil {
		return err
	}

	listen, err := net.Listen("tcp", fmt.Sprintf(":%d", fwd.localPort))
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
				if err := fwd.handleConnection(ctx, id, conn); err != nil {
					sendError(err)
				}
			}()
		}
	}()

	return awaitError(ctx, errc)
}

// ForwardWithConn handles port forwarding for a single connection.
func (fwd *PortForwarder) ForwardWithConn(ctx context.Context, conn io.ReadWriteCloser) error {
	id, err := fwd.shareRemotePort(ctx)
	if err != nil {
		conn.Close()
		return err
	}

	// Create buffered channel so that send doesn't get stuck after context cancellation.
	errc := make(chan error, 1)
	go func() {
		if err := fwd.handleConnection(ctx, id, conn); err != nil {
			errc <- err
		}
	}()
	return awaitError(ctx, errc)
}

func (fwd *PortForwarder) shareRemotePort(ctx context.Context) (channelID, error) {
	id, err := fwd.session.startSharing(ctx, fwd.name, fwd.remotePort)
	if err != nil {
		err = fmt.Errorf("failed to share remote port %d: %v", fwd.remotePort, err)
	}
	return id, nil
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
func (fwd *PortForwarder) handleConnection(ctx context.Context, id channelID, conn io.ReadWriteCloser) (err error) {
	defer safeClose(conn, &err)

	channel, err := fwd.session.openStreamingChannel(ctx, id)
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
