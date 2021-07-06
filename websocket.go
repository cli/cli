package liveshare

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
)

type websocket struct {
	session    *session
	conn       *gorillawebsocket.Conn
	readMutex  sync.Mutex
	writeMutex sync.Mutex
	reader     io.Reader
}

func newWebsocket(session *session) *websocket {
	return &websocket{session: session}
}

func (w *websocket) connect(ctx context.Context) error {
	ws, _, err := gorillawebsocket.DefaultDialer.Dial(w.session.relayURI("connect"), nil)
	if err != nil {
		return err
	}
	w.conn = ws
	return nil
}

func (w *websocket) Read(b []byte) (int, error) {
	w.readMutex.Lock()
	defer w.readMutex.Unlock()

	if w.reader == nil {
		messageType, reader, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if messageType != gorillawebsocket.BinaryMessage {
			return 0, errors.New("unexpected websocket message type")
		}

		w.reader = reader
	}

	bytesRead, err := w.reader.Read(b)
	if err != nil {
		w.reader = nil

		if err == io.EOF {
			err = nil
		}
	}

	return bytesRead, err
}

func (w *websocket) Write(b []byte) (int, error) {
	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	nextWriter, err := w.conn.NextWriter(gorillawebsocket.BinaryMessage)
	if err != nil {
		return 0, err
	}

	bytesWritten, err := nextWriter.Write(b)
	nextWriter.Close()

	return bytesWritten, err
}

func (w *websocket) Close() error {
	return w.conn.Close()
}

func (w *websocket) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *websocket) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *websocket) SetDeadline(t time.Time) error {
	if err := w.SetReadDeadline(t); err != nil {
		return err
	}

	return w.SetWriteDeadline(t)
}

func (w *websocket) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *websocket) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
