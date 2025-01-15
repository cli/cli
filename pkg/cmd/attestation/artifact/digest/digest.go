package digest

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

const (
	SHA256DigestAlgorithm = "sha256"
	SHA512DigestAlgorithm = "sha512"
)

var (
	errUnsupportedAlgorithm = fmt.Errorf("unsupported digest algorithm")
	validDigestAlgorithms   = [...]string{SHA256DigestAlgorithm, SHA512DigestAlgorithm}
)

// IsValidDigestAlgorithm returns true if the provided algorithm is supported
func IsValidDigestAlgorithm(alg string) bool {
	for _, a := range validDigestAlgorithms {
		if a == alg {
			return true
		}
	}
	return false
}

// ValidDigestAlgorithms returns a list of supported digest algorithms
func ValidDigestAlgorithms() []string {
	return validDigestAlgorithms[:]
}

func CalculateDigestWithAlgorithm(r io.Reader, alg string) (string, error) {
	var h hash.Hash
	switch alg {
	case SHA256DigestAlgorithm:
		h = sha256.New()
	case SHA512DigestAlgorithm:
		h = sha512.New()
	default:
		return "", errUnsupportedAlgorithm
	}

	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("failed to calculate digest: %v", err)
	}
	digest := h.Sum(nil)
	return hex.EncodeToString(digest), nil
}
