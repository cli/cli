package iana

import (
	"fmt"

	"github.com/weppos/publicsuffix-go/publicsuffix"
)

// ExtractSuffix returns the public suffix of the domain using only the "ICANN"
// section of the Public Suffix List database.
// If the domain does not end in a suffix that belongs to an IANA-assigned
// domain, ExtractSuffix returns an error.
func ExtractSuffix(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("Blank name argument passed to ExtractSuffix")
	}

	rule := publicsuffix.DefaultList.Find(name, &publicsuffix.FindOptions{IgnorePrivate: true, DefaultRule: nil})
	if rule == nil {
		return "", fmt.Errorf("Domain %s has no IANA TLD", name)
	}

	suffix := rule.Decompose(name)[1]

	// If the TLD is empty, it means name is actually a suffix.
	// In fact, decompose returns an array of empty strings in this case.
	if suffix == "" {
		suffix = name
	}

	return suffix, nil
}
