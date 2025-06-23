package sa

import (
	"net"
	"testing"
)

func TestIncrementIP(t *testing.T) {
	testCases := []struct {
		ip       string
		index    int
		expected string
	}{
		{"0.0.0.0", 128, "0.0.0.1"},
		{"0.0.0.255", 128, "0.0.1.0"},
		{"127.0.0.1", 128, "127.0.0.2"},
		{"1.2.3.4", 120, "1.2.4.4"},
		{"::1", 128, "::2"},
		{"2002:1001:4008::", 128, "2002:1001:4008::1"},
		{"2002:1001:4008::", 48, "2002:1001:4009::"},
		{"2002:1001:ffff::", 48, "2002:1002::"},
		{"ffff:ffff:ffff::", 48, "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
	}
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip).To16()
		actual := incrementIP(ip, tc.index)
		expectedIP := net.ParseIP(tc.expected)
		if !actual.Equal(expectedIP) {
			t.Errorf("Expected incrementIP(%s, %d) to be %s, instead got %s",
				tc.ip, tc.index, expectedIP, actual.String())
		}
	}
}

func TestIPRange(t *testing.T) {
	testCases := []struct {
		ip            string
		expectedBegin string
		expectedEnd   string
	}{
		{"28.45.45.28", "28.45.45.28", "28.45.45.29"},
		{"2002:1001:4008::", "2002:1001:4008::", "2002:1001:4009::"},
	}
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip)
		expectedBegin := net.ParseIP(tc.expectedBegin)
		expectedEnd := net.ParseIP(tc.expectedEnd)
		actualBegin, actualEnd := ipRange(ip)
		if !expectedBegin.Equal(actualBegin) || !expectedEnd.Equal(actualEnd) {
			t.Errorf("Expected ipRange(%s) to be (%s, %s), got (%s, %s)",
				tc.ip, tc.expectedBegin, tc.expectedEnd, actualBegin, actualEnd)
		}
	}
}
