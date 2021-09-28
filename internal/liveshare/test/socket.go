package livesharetest

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type socketConn struct {
	*websocket.Conn

	reader     io.Reader
	writeMutex sync.Mutex
	readMutex  sync.Mutex
}

func newSocketConn(conn *websocket.Conn) *socketConn {
	return &socketConn{Conn: conn}
}

func (s *socketConn) Read(b []byte) (int, error) {
	s.readMutex.Lock()
	defer s.readMutex.Unlock()

	if s.reader == nil {
		msgType, r, err := s.Conn.NextReader()
		if err != nil {
			return 0, fmt.Errorf("error getting next reader: %w", err)
		}
		if msgType != websocket.BinaryMessage {
			return 0, fmt.Errorf("invalid message type")
		}
		s.reader = r
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

func (s *socketConn) Write(b []byte) (int, error) {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	w, err := s.Conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, fmt.Errorf("error getting next writer: %w", err)
	}

	n, err := w.Write(b)
	if err != nil {
		return 0, fmt.Errorf("error writing: %w", err)
	}

	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("error closing writer: %w", err)
	}

	return n, nil
}

func (s *socketConn) SetDeadline(deadline time.Time) error {
	if err := s.Conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	return s.Conn.SetWriteDeadline(deadline)
}
