package shared

import (
	"testing"
	"time"
)

func TestPreciseAgo(t *testing.T) {
	const form = "2006-Jan-02 15:04:05"
	now, _ := time.Parse(form, "2021-Apr-12 14:00:00")

	cases := map[string]string{
		"2021-Apr-12 14:00:00": "0s ago",
		"2021-Apr-12 13:59:30": "30s ago",
		"2021-Apr-12 13:59:00": "1m0s ago",
		"2021-Apr-12 13:30:15": "29m45s ago",
		"2021-Apr-12 13:00:00": "1h0m0s ago",
		"2021-Apr-12 02:30:45": "11h29m15s ago",
		"2021-Apr-11 14:00:00": "24h0m0s ago",
		"2021-Apr-01 14:00:00": "264h0m0s ago",
		"2021-Mar-12 14:00:00": "Mar 12, 2021",
	}

	for createdAt, expected := range cases {
		d, _ := time.Parse(form, createdAt)
		got := preciseAgo(now, d)
		if got != expected {
			t.Errorf("expected %s but got %s for %s", expected, got, createdAt)
		}
	}
}
