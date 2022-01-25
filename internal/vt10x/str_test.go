package vt10x

import (
	"testing"
)

func TestSTRParse(t *testing.T) {
	var str strEscape
	str.reset()
	str.buf = []rune("0;some text")
	str.parse()
	if str.arg(0, 17) != 0 || str.argString(1, "") != "some text" {
		t.Fatal("STR parse mismatch")
	}
}
