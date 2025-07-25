package iana

import "testing"

func TestExtractSuffix_Valid(t *testing.T) {
	testCases := []struct {
		domain, want string
	}{
		// TLD with only 1 rule.
		{"biz", "biz"},
		{"domain.biz", "biz"},
		{"b.domain.biz", "biz"},

		// The relevant {kobe,kyoto}.jp rules are:
		// jp
		// *.kobe.jp
		// !city.kobe.jp
		// kyoto.jp
		// ide.kyoto.jp
		{"jp", "jp"},
		{"kobe.jp", "jp"},
		{"c.kobe.jp", "c.kobe.jp"},
		{"b.c.kobe.jp", "c.kobe.jp"},
		{"a.b.c.kobe.jp", "c.kobe.jp"},
		{"city.kobe.jp", "kobe.jp"},
		{"www.city.kobe.jp", "kobe.jp"},
		{"kyoto.jp", "kyoto.jp"},
		{"test.kyoto.jp", "kyoto.jp"},
		{"ide.kyoto.jp", "ide.kyoto.jp"},
		{"b.ide.kyoto.jp", "ide.kyoto.jp"},
		{"a.b.ide.kyoto.jp", "ide.kyoto.jp"},

		// Domain with a private public suffix should return the ICANN public suffix.
		{"foo.compute-1.amazonaws.com", "com"},
		// Domain equal to a private public suffix should return the ICANN public
		// suffix.
		{"cloudapp.net", "net"},
	}

	for _, tc := range testCases {
		got, err := ExtractSuffix(tc.domain)
		if err != nil {
			t.Errorf("%q: returned error", tc.domain)
			continue
		}
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.domain, got, tc.want)
		}
	}
}

func TestExtractSuffix_Invalid(t *testing.T) {
	testCases := []string{
		"",
		"example",
		"example.example",
	}

	for _, tc := range testCases {
		_, err := ExtractSuffix(tc)
		if err == nil {
			t.Errorf("%q: expected err, got none", tc)
		}
	}
}
