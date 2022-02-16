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

type requestHandler struct {
	eventHandlersMu sync.RWMutex
	eventHandlers   map[string]chan []byte
}

func newRequestHandler() *requestHandler {
	return &requestHandler{eventHandlers: make(map[string]chan []byte)}
}

func (r *requestHandler) registerEvent(eventName string) chan []byte {
	r.eventHandlersMu.Lock()
	defer r.eventHandlersMu.Unlock()

	if ch, ok := r.eventHandlers[eventName]; ok {
		return ch
	}

	ch := make(chan []byte)
	r.eventHandlers[eventName] = ch
	return ch
}

func (r *requestHandler) eventHandler(eventName string) chan []byte {
	r.eventHandlersMu.RLock()
	defer r.eventHandlersMu.RUnlock()

	return r.eventHandlers[eventName]
}

func (r *requestHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	fmt.Println(req.Method)
	if req.Params != nil {
		fmt.Println(string(*req.Params))
	}
	handler := r.eventHandler(req.Method)
	if handler == nil {
		return // noop
	}

	handler <- *req.Params
}
