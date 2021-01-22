package shared

import "testing"

func Test_truncateLabels(t *testing.T) {
	got := truncateLabels(12, "(one, two, three)")
	expected := "(one, tw...)"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	if truncateLabels(10, "") != "" {
		t.Error("blank value error")
	}
}
