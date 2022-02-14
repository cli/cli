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
	conn io.ReadWriteCloser

	eventHandlersMu sync.RWMutex
	eventHandlers   map[string]chan []byte
}

func newRPCClient(conn io.ReadWriteCloser) *rpcClient {
	return &rpcClient{conn: conn, eventHandlers: make(map[string]chan []byte)}
}

func (r *rpcClient) connect(ctx context.Context) {
	stream := jsonrpc2.NewBufferedStream(r.conn, jsonrpc2.VSCodeObjectCodec{})
	r.Conn = jsonrpc2.NewConn(ctx, stream, newRequestHandler(r))
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

func (r *rpcClient) registerEventHandler(eventName string) chan []byte {
	r.eventHandlersMu.Lock()
	defer r.eventHandlersMu.Unlock()

	if ch, ok := r.eventHandlers[eventName]; ok {
		return ch
	}

	ch := make(chan []byte)
	r.eventHandlers[eventName] = ch
	return ch
}

func (r *rpcClient) eventHandler(eventName string) chan []byte {
	r.eventHandlersMu.RLock()
	defer r.eventHandlersMu.RUnlock()

	return r.eventHandlers[eventName]
}

type requestHandler struct {
	rpcClient *rpcClient
}

func newRequestHandler(rpcClient *rpcClient) *requestHandler {
	return &requestHandler{rpcClient: rpcClient}
}

func (e *requestHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	handler := e.rpcClient.eventHandler(req.Method)
	if handler == nil {
		return // noop
	}

	select {
	case handler <- *req.Params:
	default:
		// event handler
	}
}
