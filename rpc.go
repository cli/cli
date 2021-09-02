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

type rpcHandlerFunc = func(*jsonrpc2.Request)

type rpcHandler struct {
	handlersMu sync.Mutex
	handlers   map[string][]rpcHandlerFunc
}

func newRPCHandler() *rpcHandler {
	return &rpcHandler{
		handlers: make(map[string][]rpcHandlerFunc),
	}
}

// registerEventHandler registers a handler for the specified event.
// After the next occurrence of the event, the handler will be called,
// once, in its own goroutine.
func (r *rpcHandler) registerEventHandler(eventMethod string, h rpcHandlerFunc) {
	r.handlersMu.Lock()
	r.handlers[eventMethod] = append(r.handlers[eventMethod], h)
	r.handlersMu.Unlock()
}

// Handle calls all registered handlers for the request, concurrently, each in its own goroutine.
func (r *rpcHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r.handlersMu.Lock()
	handlers := r.handlers[req.Method]
	r.handlers[req.Method] = nil
	r.handlersMu.Unlock()

	for _, h := range handlers {
		go h(req)
	}
}
