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
	conn           io.ReadWriteCloser
	requestHandler *requestHandler
}

func newRPCClient(conn io.ReadWriteCloser) *rpcClient {
	return &rpcClient{conn: conn, requestHandler: newRequestHandler()}
}

func (r *rpcClient) connect(ctx context.Context) {
	stream := jsonrpc2.NewBufferedStream(r.conn, jsonrpc2.VSCodeObjectCodec{})
	r.Conn = jsonrpc2.NewConn(ctx, stream, r.requestHandler)
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

type handlerFn func(req *jsonrpc2.Request)

type requestHandler struct {
	handlersMu sync.RWMutex
	handlers   map[string][]handlerFn
}

func newRequestHandler() *requestHandler {
	return &requestHandler{handlers: make(map[string][]handlerFn)}
}

func (r *requestHandler) register(requestType string, handler handlerFn) {
	r.handlersMu.Lock()
	defer r.handlersMu.Unlock()

	if _, ok := r.handlers[requestType]; !ok {
		r.handlers[requestType] = []handlerFn{}
	}

	r.handlers[requestType] = append(r.handlers[requestType], handler)
}

func (r *requestHandler) handlerFn(requestType string) []handlerFn {
	r.handlersMu.RLock()
	defer r.handlersMu.RUnlock()

	return r.handlers[requestType]
}

func (r *requestHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	for _, handler := range r.handlerFn(req.Method) {
		go handler(req)
	}
}
