package liveshare

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Adapter struct {
	conn       *websocket.Conn
	readMutex  sync.Mutex
	writeMutex sync.Mutex
	reader     io.Reader
}

func NewAdapter(conn *websocket.Conn) *Adapter {
	return &Adapter{
		conn: conn,
	}
}

func (a *Adapter) Read(b []byte) (int, error) {
	// Read() can be called concurrently, and we mutate some internal state here
	a.readMutex.Lock()
	defer a.readMutex.Unlock()

	if a.reader == nil {
		messageType, reader, err := a.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if messageType != websocket.BinaryMessage {
			return 0, errors.New("unexpected websocket message type")
		}

		a.reader = reader
	}

	bytesRead, err := a.reader.Read(b)
	if err != nil {
		a.reader = nil

		// EOF for the current Websocket frame, more will probably come so..
		if err == io.EOF {
			// .. we must hide this from the caller since our semantics are a
			// stream of bytes across many frames
			err = nil
		}
	}

	return bytesRead, err
}

func (a *Adapter) Write(b []byte) (int, error) {
	a.writeMutex.Lock()
	defer a.writeMutex.Unlock()

	nextWriter, err := a.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}

	bytesWritten, err := nextWriter.Write(b)
	nextWriter.Close()

	return bytesWritten, err
}

func (a *Adapter) Close() error {
	return a.conn.Close()
}

func (a *Adapter) LocalAddr() net.Addr {
	return a.conn.LocalAddr()
}

func (a *Adapter) RemoteAddr() net.Addr {
	return a.conn.RemoteAddr()
}

func (a *Adapter) SetDeadline(t time.Time) error {
	if err := a.SetReadDeadline(t); err != nil {
		return err
	}

	return a.SetWriteDeadline(t)
}

func (a *Adapter) SetReadDeadline(t time.Time) error {
	return a.conn.SetReadDeadline(t)
}

func (a *Adapter) SetWriteDeadline(t time.Time) error {
	return a.conn.SetWriteDeadline(t)
}
