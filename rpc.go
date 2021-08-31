package liveshare

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/sourcegraph/jsonrpc2"
)

type rpcClient struct {
	*jsonrpc2.Conn
	conn    io.ReadWriteCloser
	handler *rpcHandler
}

func newRPCClient(conn io.ReadWriteCloser) *rpcClient {
	return &rpcClient{conn: conn, handler: newRPCHandler()}
}

func (r *rpcClient) connect(ctx context.Context) {
	stream := jsonrpc2.NewBufferedStream(r.conn, jsonrpc2.VSCodeObjectCodec{})
	r.Conn = jsonrpc2.NewConn(ctx, stream, r.handler)
}

func (r *rpcClient) do(ctx context.Context, method string, args, result interface{}) error {
	waiter, err := r.Conn.DispatchCall(ctx, method, args)
	if err != nil {
		return fmt.Errorf("error dispatching %q call: %v", method, err)
	}

	return waiter.Wait(ctx, result)
}

type rpcHandler struct {
	mutex         sync.Mutex
	eventHandlers map[string][]chan *jsonrpc2.Request
}

func newRPCHandler() *rpcHandler {
	return &rpcHandler{
		eventHandlers: make(map[string][]chan *jsonrpc2.Request),
	}
}

// TODO: document obligations around chan. It appears to be used for at most one request.
func (r *rpcHandler) registerEventHandler(eventMethod string) <-chan *jsonrpc2.Request {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	ch := make(chan *jsonrpc2.Request)
	r.eventHandlers[eventMethod] = append(r.eventHandlers[eventMethod], ch)
	return ch
}

func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r.mutex.Lock()
	handlers := r.eventHandlers[req.Method]
	r.eventHandlers[req.Method] = nil
	r.mutex.Unlock()

	if len(handlers) > 0 {
		go func() {
			// Broadcast the request to each handler in sequence.
			// TODO rethink this. needs function call.
			for _, handler := range handlers {
				select {
				case handler <- req:
				case <-ctx.Done():
					// TODO: ctx.Err
					break
				}
			}
		}()
	}
}
