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
