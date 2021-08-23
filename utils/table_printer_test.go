package utils

import (
	"bytes"
	"testing"
)

func Test_ttyTablePrinter_truncate(t *testing.T) {
	buf := bytes.Buffer{}
	tp := &ttyTablePrinter{
		out:      &buf,
		maxWidth: 5,
	}

	tp.AddField("1", nil, nil)
	tp.AddField("hello", nil, nil)
	tp.EndRow()
	tp.AddField("2", nil, nil)
	tp.AddField("world", nil, nil)
	tp.EndRow()

	err := tp.Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "1  he\n2  wo\n"
	if buf.String() != expected {
		t.Errorf("expected: %q, got: %q", expected, buf.String())
	}
}
