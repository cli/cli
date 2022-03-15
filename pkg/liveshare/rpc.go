package liveshare

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/sourcegraph/jsonrpc2"
)

type rpcClient struct {
	*jsonrpc2.Conn
	conn       io.ReadWriteCloser
	handlersMu sync.Mutex
	handlers   map[string][]*handlerWrapper
}

func newRPCClient(conn io.ReadWriteCloser) *rpcClient {
	return &rpcClient{conn: conn, handlers: make(map[string][]*handlerWrapper)}
}

func (r *rpcClient) connect(ctx context.Context) {
	stream := jsonrpc2.NewBufferedStream(r.conn, jsonrpc2.VSCodeObjectCodec{})
	r.Conn = jsonrpc2.NewConn(ctx, stream, r)
}

func (r *rpcClient) do(ctx context.Context, method string, args, result interface{}) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, method)
	defer span.Finish()

	waiter, err := r.Conn.DispatchCall(ctx, method, args)
	if err != nil {
		return fmt.Errorf("error dispatching %q call: %w", method, err)
	}

	// timeout for waiter in case a connection cannot be made
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	return waiter.Wait(waitCtx, result)
}

type handler func(conn *jsonrpc2.Conn, req *jsonrpc2.Request)

type handlerWrapper struct {
	fn handler
}

func (r *rpcClient) register(requestType string, fn handler) func() {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	h := &handlerWrapper{fn: fn}
	r.handlers[requestType] = append(r.handlers[requestType], h)

	return func() {
		r.deregister(requestType, h)
	}
}

func (r *rpcClient) deregister(requestType string, handler *handlerWrapper) {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	handlers := r.handlers[requestType]
	for i, h := range handlers {
		if h == handler {
			// Swap h with last element and pop.
			last := len(handlers) - 1
			handlers[i], handlers[last] = handlers[last], nil
			r.handlers[requestType] = handlers[:last]
			break
		}
	}
}

func (r *rpcClient) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	for _, handler := range r.handlers[req.Method] {
		go handler.fn(conn, req)
	}
}
