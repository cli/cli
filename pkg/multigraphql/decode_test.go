package multigraphql

import (
	"bytes"
	"testing"
)

func TestDecode(t *testing.T) {
	buf := bytes.NewBufferString(`
	{ "extensions": [],
	  "data": {
		"multi_000": { "world": true },
		"multi_001": { "machines": "are learning" }
	} }
	`)

	hello := struct {
		World bool
	}{}
	ai := struct {
		Machines string
	}{}

	err := Decode(buf, []interface{}{&hello, &ai})
	if err != nil {
		t.Fatalf("got error: %v", err)
	}

	if !hello.World {
		t.Errorf("expected World to be true")
	}
	if ai.Machines != "are learning" {
		t.Errorf("expected machines to be learning, got %q", ai.Machines)
	}
}

func TestDecode_errors(t *testing.T) {
	buf := bytes.NewBufferString(`
	{ "extensions": [],
	  "errors": [
		{ "message": "boom" },
		{ "message": "shutting down" }
	] }
	`)

	hello := struct {
		World bool
	}{}

	err := Decode(buf, []interface{}{&hello})
	if err == nil || err.Error() != "GraphQL error: boom; shutting down" {
		t.Fatalf("got error: %v", err)
	}
}
