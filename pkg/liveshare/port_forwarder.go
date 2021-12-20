package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/opentracing/opentracing-go"
)

// A PortForwarder forwards TCP traffic over a Live Share session from a port on a remote
// container to a local destination such as a network port or Go reader/writer.
type PortForwarder struct {
	session    *Session
	name       string
	remotePort int
	keepAlive  bool
}

// NewPortForwarder returns a new PortForwarder for the specified
// remote port and Live Share session. The name describes the purpose
// of the remote port or service. The keepAlive flag indicates whether
// the session should be kept alive with port forwarding traffic.
func NewPortForwarder(session *Session, name string, remotePort int, keepAlive bool) *PortForwarder {
	return &PortForwarder{
		session:    session,
		name:       name,
		remotePort: remotePort,
		keepAlive:  keepAlive,
	}
}

// ForwardToListener forwards traffic between the container's remote
// port and a local port, which must already be listening for
// connections. (Accepting a listener rather than a port number avoids
// races against other processes opening ports, and against a client
// connecting to the socket prematurely.)
//
// ForwardToListener accepts and handles connections on the local port
// until it encounters the first error, which may include context
// cancellation. Its error result is always non-nil. The caller is
// responsible for closing the listening port.
func (fwd *PortForwarder) ForwardToListener(ctx context.Context, listen net.Listener) (err error) {
	id, err := fwd.shareRemotePort(ctx)
	if err != nil {
		return err
	}

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

// Forward forwards traffic between the container's remote port and
// the specified read/write stream. On return, the stream is closed.
func (fwd *PortForwarder) Forward(ctx context.Context, conn io.ReadWriteCloser) error {
	id, err := fwd.shareRemotePort(ctx)
	if err != nil {
		conn.Close()
		return err
	}

	// Create buffered channel so that send doesn't get stuck after context cancellation.
	errc := make(chan error, 1)
	go func() {
		errc <- fwd.handleConnection(ctx, id, conn)
	}()
	return awaitError(ctx, errc)
}

func (fwd *PortForwarder) shareRemotePort(ctx context.Context) (channelID, error) {
	id, err := fwd.session.startSharing(ctx, fwd.name, fwd.remotePort)
	if err != nil {
		err = fmt.Errorf("failed to share remote port %d: %w", fwd.remotePort, err)
	}
	return id, err
}

func awaitError(ctx context.Context, errc <-chan error) error {
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err() // canceled
	}
}

// trafficMonitor implements io.Reader. It keeps the session alive by notifying
// it of the traffic type during Read operations.
type trafficMonitor struct {
	reader io.Reader

	session     *Session
	trafficType string
}

// newTrafficMonitor returns a new trafficMonitor for the specified
// session and traffic type. It wraps the provided io.Reader with its own
// Read method.
func newTrafficMonitor(reader io.Reader, session *Session, trafficType string) *trafficMonitor {
	return &trafficMonitor{reader, session, trafficType}
}

func (t *trafficMonitor) Read(p []byte) (n int, err error) {
	t.session.keepAlive(t.trafficType)
	return t.reader.Read(p)
}

// handleConnection handles forwarding for a single accepted connection, then closes it.
func (fwd *PortForwarder) handleConnection(ctx context.Context, id channelID, conn io.ReadWriteCloser) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PortForwarder.handleConnection")
	defer span.Finish()

	defer safeClose(conn, &err)

	channel, err := fwd.session.openStreamingChannel(ctx, id)
	if err != nil {
		return fmt.Errorf("error opening streaming channel for new connection: %w", err)
	}
	// Ideally we would call safeClose again, but (*ssh.channel).Close
	// appears to have a bug that causes it return io.EOF spuriously
	// if its peer closed first; see github.com/golang/go/issues/38115.
	defer func() {
		closeErr := channel.Close()
		if err == nil && closeErr != io.EOF {
			err = closeErr
		}
	}()

	// bi-directional copy of data.
	errs := make(chan error, 2)
	copyConn := func(w io.Writer, r io.Reader) {
		_, err := io.Copy(w, r)
		errs <- err
	}

	var (
		channelReader io.Reader = channel
		connReader    io.Reader = conn
	)

	// If the forwader has been configured to keep the session alive
	// it will monitor the I/O and notify the session of the traffic.
	if fwd.keepAlive {
		channelReader = newTrafficMonitor(channelReader, fwd.session, "output")
		connReader = newTrafficMonitor(connReader, fwd.session, "input")
	}

	go copyConn(conn, channelReader)
	go copyConn(channel, connReader)

	// Wait until context is cancelled or both copies are done.
	// Discard errors from io.Copy; they should not cause (e.g.) ForwardToListener to fail.
	// TODO: how can we proxy errors from Copy so that each peer can distinguish an error from a short file?
	for i := 0; ; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-errs:
			i++
			if i == 2 {
				return nil
			}
		}
	}
}

// safeClose reports the error (to *err) from closing the stream only
// if no other error was previously reported.
func safeClose(closer io.Closer, err *error) {
	closeErr := closer.Close()
	if *err == nil {
		*err = closeErr
	}
}
