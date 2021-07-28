package liveshare

import "testing"

func TestConnectionValid(t *testing.T) {
	conn := Connection{"sess-id", "sess-token", "sas", "endpoint"}
	if err := conn.validate(); err != nil {
		t.Error(err)
	}
}

func TestConnectionInvalid(t *testing.T) {
	conn := Connection{"", "sess-token", "sas", "endpoint"}
	if err := conn.validate(); err == nil {
		t.Error(err)
	}
	conn = Connection{"sess-id", "", "sas", "endpoint"}
	if err := conn.validate(); err == nil {
		t.Error(err)
	}
	conn = Connection{"sess-id", "sess-token", "", "endpoint"}
	if err := conn.validate(); err == nil {
		t.Error(err)
	}
	conn = Connection{"sess-id", "sess-token", "sas", ""}
	if err := conn.validate(); err == nil {
		t.Error(err)
	}
	conn = Connection{"", "", "", ""}
	if err := conn.validate(); err == nil {
		t.Error(err)
	}
}

func TestConnectionURI(t *testing.T) {
	conn := Connection{"sess-id", "sess-token", "sas", "sb://endpoint/.net/liveshare"}
	uri := conn.uri("connect")
	if uri != "wss://endpoint/.net:443/$hc/liveshare?sb-hc-action=connect&sb-hc-token=sas" {
		t.Errorf("uri is not correct, got: '%v'", uri)
	}
}
