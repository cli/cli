package test

import (
	"io"
	"net"
)

type Channel struct {
	conn net.Conn
}

func (c *Channel) Read(data []byte) (int, error) {
	return c.conn.Read(data)
}

func (c *Channel) Write(data []byte) (int, error) {
	return c.conn.Write(data)
}

func (c *Channel) Close() error {
	return c.conn.Close()
}

func (c *Channel) CloseWrite() error {
	return nil
}

func (c *Channel) SendRequest(name string, wantReply bool, payload []byte) (bool, error) {
	return false, nil
}

func (c *Channel) Stderr() io.ReadWriter {
	return nil
}
