package liveshare

import (
	"context"
	"testing"
)

func TestBadOptions(t *testing.T) {
	goodOptions := Options{
		SessionID:     "sess-id",
		SessionToken:  "sess-token",
		RelaySAS:      "sas",
		RelayEndpoint: "endpoint",
	}

	opts := goodOptions
	opts.SessionID = ""
	checkBadOptions(t, opts)

	opts = goodOptions
	opts.SessionToken = ""
	checkBadOptions(t, opts)

	opts = goodOptions
	opts.RelaySAS = ""
	checkBadOptions(t, opts)

	opts = goodOptions
	opts.RelayEndpoint = ""
	checkBadOptions(t, opts)

	opts = Options{}
	checkBadOptions(t, opts)
}

func checkBadOptions(t *testing.T, opts Options) {
	if _, err := Connect(context.Background(), opts); err == nil {
		t.Errorf("Connect(%+v): no error", opts)
	}
}

func TestOptionsURI(t *testing.T) {
	opts := Options{
		SessionID:     "sess-id",
		SessionToken:  "sess-token",
		RelaySAS:      "sas",
		RelayEndpoint: "sb://endpoint/.net/liveshare",
	}
	uri, err := opts.uri("connect")
	if err != nil {
		t.Fatal(err)
	}
	if uri != "wss://endpoint/.net:443/$hc/liveshare?sb-hc-action=connect&sb-hc-token=sas" {
		t.Errorf("uri is not correct, got: '%v'", uri)
	}
}
