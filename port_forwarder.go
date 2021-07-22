package liveshare

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"

	"golang.org/x/crypto/ssh"
)

type PortForwarder struct {
	client   *Client
	server   *Server
	port     int
	channels []ssh.Channel
}

func NewPortForwarder(client *Client, server *Server, port int) *PortForwarder {
	return &PortForwarder{client, server, port, []ssh.Channel{}}
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

	// clean up after ourselves

	return nil
}

func (l *PortForwarder) handleConnection(ctx context.Context, conn net.Conn) {
	channel, err := l.client.openStreamingChannel(ctx, l.server.streamName, l.server.streamCondition)
	if err != nil {
		log.Println("errrr handle Connect")
		log.Println(err) // TODO(josebalius) handle this somehow
	}
	l.channels = append(l.channels, channel)

	copyConn := func(writer io.Writer, reader io.Reader) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			channel.Close()
			conn.Close()
		}
	}

	go copyConn(conn, channel)
	go copyConn(channel, conn)
}
