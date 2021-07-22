package liveshare

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
)

type PortForwarder struct {
	client *Client
	server *Server
	port   int
	errCh  chan error
}

func NewPortForwarder(client *Client, server *Server, port int) *PortForwarder {
	return &PortForwarder{
		client: client,
		server: server,
		port:   port,
		errCh:  make(chan error),
	}
}

func (l *PortForwarder) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(l.port))
	if err != nil {
		return fmt.Errorf("error listening on tcp port: %v", err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("error accepting incoming connection: %v", err)
		}

		go l.handleConnection(ctx, conn)
	}

	return nil
}

func (l *PortForwarder) handleConnection(ctx context.Context, conn net.Conn) {
	channel, err := l.client.openStreamingChannel(ctx, l.server.streamName, l.server.streamCondition)
	if err != nil {
		l.errCh <- fmt.Errorf("error opening streaming channel for new connection: %v", err)
		return
	}

	copyConn := func(writer io.Writer, reader io.Reader) {
		if _, err := io.Copy(writer, reader); err != nil {
			channel.Close()
			conn.Close()
		}
	}

	go copyConn(conn, channel)
	go copyConn(channel, conn)
}
