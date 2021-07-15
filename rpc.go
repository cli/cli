package liveshare

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

type rpc struct {
	*jsonrpc2.Conn
	conn    io.ReadWriteCloser
	handler *rpcHandler
}

func newRPC(conn io.ReadWriteCloser) *rpc {
	return &rpc{conn: conn, handler: newRPCHandler()}
}

func (r *rpc) connect(ctx context.Context) {
	stream := jsonrpc2.NewBufferedStream(r.conn, jsonrpc2.VSCodeObjectCodec{})
	r.Conn = jsonrpc2.NewConn(ctx, stream, r.handler)
}

func (r *rpc) do(ctx context.Context, method string, args interface{}, result interface{}) error {
	waiter, err := r.Conn.DispatchCall(ctx, method, args)
	if err != nil {
		return fmt.Errorf("error on dispatch call: %v", err)
	}

	// caller doesn't care about result, so lets ignore it
	if result == nil {
		return nil
	}

	return waiter.Wait(ctx, result)
}

type rpcHandler struct {
	mutex         sync.RWMutex
	eventHandlers map[string][]chan *jsonrpc2.Request
}

func newRPCHandler() *rpcHandler {
	return &rpcHandler{
		eventHandlers: make(map[string][]chan *jsonrpc2.Request),
	}
}

func (r *rpcHandler) registerEventHandler(eventMethod string) <-chan *jsonrpc2.Request {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ch := make(chan *jsonrpc2.Request)
	if _, ok := r.eventHandlers[eventMethod]; !ok {
		r.eventHandlers[eventMethod] = []chan *jsonrpc2.Request{ch}
	} else {
		r.eventHandlers[eventMethod] = append(r.eventHandlers[eventMethod], ch)
	}
	return ch
}

func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if handlers, ok := r.eventHandlers[req.Method]; ok {
		go func() {
			for _, handler := range handlers {
				select {
				case handler <- req:
				case <-ctx.Done():
					break
				}
			}

			r.eventHandlers[req.Method] = []chan *jsonrpc2.Request{}
		}()
	} else {
		// TODO(josebalius): Handle
	}
}
