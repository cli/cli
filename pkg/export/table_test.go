package export

import (
	"strings"
	"testing"
)

func Test_Row_withoutTable(t *testing.T) {
	formatter := &tableFormatter{
		current: nil,
		writer:  &strings.Builder{},
	}
	if _, err := formatter.Row("col"); err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_EndTable_withoutTable(t *testing.T) {
	formatter := &tableFormatter{
		current: nil,
		writer:  &strings.Builder{},
	}
	if _, err := formatter.EndTable(); err == nil {
		t.Errorf("expected error, got nil")
	}
}
