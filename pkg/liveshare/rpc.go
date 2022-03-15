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
	handlers   map[string][]*handlerSt
}

func newRPCClient(conn io.ReadWriteCloser) *rpcClient {
	return &rpcClient{conn: conn, handlers: make(map[string][]*handlerSt)}
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

type handlerFn func(conn *jsonrpc2.Conn, req *jsonrpc2.Request)

type handlerSt struct {
	fn handlerFn
}

func (r *rpcClient) register(requestType string, fn handlerFn) func() {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	h := &handlerSt{fn: fn}
	r.handlers[requestType] = append(r.handlers[requestType], h)

	return func() {
		r.deregister(requestType, h)
	}
}

func (r *rpcClient) deregister(requestType string, handler *handlerSt) {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	if handlers, ok := r.handlers[requestType]; ok {
		newHandlers := []*handlerSt{}
		for _, h := range handlers {
			if h != handler {
				newHandlers = append(newHandlers, h)
			}
		}
		r.handlers[requestType] = newHandlers

		if len(r.handlers[requestType]) == 0 {
			delete(r.handlers, requestType)
		}
	}
}

func (r *rpcClient) getHandlers(requestType string) []*handlerSt {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	return r.handlers[requestType]
}

func (r *rpcClient) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	for _, handler := range r.getHandlers(req.Method) {
		go handler.fn(conn, req)
	}
}
