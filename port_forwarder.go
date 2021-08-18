package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
)

// A PortForwader can forward ports from a remote liveshare host to localhost
type PortForwarder struct {
	client *Client
	server *Server
	port   int
	errCh  chan error
}

// NewPortForwarder creates a new PortForwader with a given client, server and port
func NewPortForwarder(client *Client, server *Server, port int) *PortForwarder {
	return &PortForwarder{
		client: client,
		server: server,
		port:   port,
		errCh:  make(chan error),
	}
}

// Start is a method to start forwarding the server to a localhost port
func (l *PortForwarder) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(l.port))
	if err != nil {
		return fmt.Errorf("error listening on tcp port: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				l.errCh <- fmt.Errorf("error accepting incoming connection: %v", err)
			}

			go l.handleConnection(ctx, conn)
		}
	}()

	select {
	case err := <-l.errCh:
		return err
	case <-ctx.Done():
		return ln.Close()
	}

	return nil
}

func (l *PortForwarder) StartWithConn(ctx context.Context, conn io.ReadWriteCloser) error {
	go l.handleConnection(ctx, conn)
	return <-l.errCh
}

func (l *PortForwarder) handleConnection(ctx context.Context, conn io.ReadWriteCloser) {
	channel, err := l.client.openStreamingChannel(ctx, l.server.streamName, l.server.streamCondition)
	if err != nil {
		l.errCh <- fmt.Errorf("error opening streaming channel for new connection: %v", err)
		return
	}

	copyConn := func(writer io.Writer, reader io.Reader) {
		if _, err := io.Copy(writer, reader); err != nil {
			channel.Close()
			conn.Close()
			if err != io.EOF {
				l.errCh <- fmt.Errorf("tunnel connection: %v", err)
			}
		}
	}

	go copyConn(conn, channel)
	go copyConn(channel, conn)
}
