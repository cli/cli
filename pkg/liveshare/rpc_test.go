package liveshare

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestRequestHandler(t *testing.T) {
	r, w := net.Pipe()
	client := newRPCClient(r)

	ctx := context.Background()
	client.connect(ctx)

	type params struct {
		Data string `json:"data"`
	}

	ev := client.registerEventHandler("testEvent")
	done := make(chan error)
	go func() {
		b := <-ev
		var receivedParams params
		if err := json.Unmarshal(b, &receivedParams); err != nil {
			done <- err
			return
		}
		if receivedParams.Data != "test" {
			done <- fmt.Errorf("expected test, got %q", receivedParams.Data)
		}
		done <- nil
	}()

	go func() {
		codec := jsonrpc2.VSCodeObjectCodec{}
		type message struct {
			Method string `json:"method"`
			Params params `json:"params"`
		}
		codec.WriteObject(w, message{
			Method: "testEvent",
			Params: params{"test"},
		})
	}()

	err := <-done
	if err != nil {
		t.Fatal(err)
	}
}
