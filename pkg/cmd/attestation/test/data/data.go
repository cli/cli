package data

import (
	_ "embed"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	sgData "github.com/sigstore/sigstore-go/pkg/testing/data"
)

//go:embed sigstore-js-2.1.0-bundle.json
var SigstoreBundleRaw []byte

// SigstoreBundle returns a test *sigstore.Bundle
func SigstoreBundle(t *testing.T) *bundle.Bundle {
	return sgData.TestBundle(t, SigstoreBundleRaw)
}
