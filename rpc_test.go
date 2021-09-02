package liveshare

import (
	"context"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

func TestRPCHandlerEvents(t *testing.T) {
	rpcHandler := newRPCHandler()
	eventCh := rpcHandler.registerEventHandler("somethingHappened")
	go func() {
		time.Sleep(1 * time.Second)
		rpcHandler.Handle(context.Background(), nil, &jsonrpc2.Request{Method: "somethingHappened"})
	}()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(5*time.Second))
	defer cancel()
	select {
	case event := <-eventCh:
		if event.Method != "somethingHappened" {
			t.Error("event.Method is not the expect value")
		}
	case <-ctx.Done():
		t.Error("Test time out")
	}
}
