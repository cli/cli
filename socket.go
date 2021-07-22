package liveshare

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type socket struct {
	addr       string
	conn       *websocket.Conn
	readMutex  sync.Mutex
	writeMutex sync.Mutex
	reader     io.Reader
}

func newSocket(clientConn Connection) *socket {
	return &socket{addr: clientConn.uri("connect")}
}

func (s *socket) connect(ctx context.Context) error {
	ws, _, err := websocket.DefaultDialer.Dial(s.addr, nil)
	if err != nil {
		return err
	}
	s.conn = ws
	return nil
}

func (s *socket) Read(b []byte) (int, error) {
	s.readMutex.Lock()
	defer s.readMutex.Unlock()

	if s.reader == nil {
		messageType, reader, err := s.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if messageType != websocket.BinaryMessage {
			return 0, errors.New("unexpected websocket message type")
		}

		s.reader = reader
	}

	bytesRead, err := s.reader.Read(b)
	if err != nil {
		s.reader = nil

		if err == io.EOF {
			err = nil
		}
	}

	return bytesRead, err
}

func (s *socket) Write(b []byte) (int, error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	nextWriter, err := s.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}

	bytesWritten, err := nextWriter.Write(b)
	nextWriter.Close()

	return bytesWritten, err
}

func (s *socket) Close() error {
	return s.conn.Close()
}

func (s *socket) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *socket) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

func (s *socket) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}

	return s.SetWriteDeadline(t)
}

func (s *socket) SetReadDeadline(t time.Time) error {
	return s.conn.SetReadDeadline(t)
}

func (s *socket) SetWriteDeadline(t time.Time) error {
	return s.conn.SetWriteDeadline(t)
}
