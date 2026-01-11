package artifact

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/digest"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
)

type artifactType int

const (
	ociArtifactType artifactType = iota
	fileArtifactType
	digestArtifactType
)

// DigestedArtifact abstracts the software artifact being verified
type DigestedArtifact struct {
	URL       string
	digest    string
	digestAlg string
	nameRef   name.Reference
}

func normalizeReference(reference string, pathSeparator rune) (normalized string, artifactType artifactType, err error) {
	lowerRef := strings.ToLower(reference)
	switch {
	case strings.HasPrefix(reference, "oci://"):
		return reference[6:], ociArtifactType, nil
	case strings.HasPrefix(lowerRef, "sha256:"), strings.HasPrefix(lowerRef, "sha512:"):
		// digest is normalized to lowercase.
		return lowerRef, digestArtifactType, nil
	case strings.HasPrefix(reference, "file://"):
		uri, err := url.ParseRequestURI(reference)
		if err != nil {
			return "", 0, fmt.Errorf("failed to parse reference URI: %v", err)
		}
		var path string
		if pathSeparator == '/' {
			// Unix paths use forward slashes like URIs, so no need to modify
			path = uri.Path
		} else {
			// Windows paths should be normalized to use backslashes
			path = strings.ReplaceAll(uri.Path, "/", string(pathSeparator))
			// Remove leading slash from Windows paths if present
			if strings.HasPrefix(path, string(pathSeparator)) {
				path = path[1:]
			}
		}
		return filepath.Clean(path), fileArtifactType, nil
	}
	// Treat any other reference as a local file path
	return filepath.Clean(reference), fileArtifactType, nil
}

func NewDigestedArtifactForRelease(d string, digestAlg string) (artifact *DigestedArtifact) {
	return &DigestedArtifact{
		digest:    d,
		digestAlg: digestAlg,
	}
}

// parseDigestReference parses a digest reference like "sha256:abc123" and returns
// the algorithm and digest value. Returns an error if the format is invalid.
func parseDigestReference(reference string) (alg, digestValue string, err error) {
	reference = strings.ToLower(reference)
	parts := strings.SplitN(reference, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid digest format, expected 'algorithm:digest'")
	}

	alg = parts[0]
	digestValue = parts[1]

	if !digest.IsValidDigestAlgorithm(alg) {
		return "", "", fmt.Errorf("unsupported digest algorithm: %s", alg)
	}

	var expectedLen int
	switch alg {
	case "sha256":
		expectedLen = 64
	case "sha512":
		expectedLen = 128
	default:
		// For any other supported algorithms, we currently don't enforce a length,
		// but we still validate that the digest is hexadecimal.
	}

	if expectedLen > 0 && len(digestValue) != expectedLen {
		return "", "", fmt.Errorf("invalid %s digest: expected %d hexadecimal characters, got %d", alg, expectedLen, len(digestValue))
	}

	for i := 0; i < len(digestValue); i++ {
		c := digestValue[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", "", fmt.Errorf("invalid %s digest: digest must be a hexadecimal string", alg)
		}
	}
	return alg, digestValue, nil
}

func NewDigestedArtifact(client oci.Client, reference, digestAlg string) (artifact *DigestedArtifact, err error) {
	normalized, artType, err := normalizeReference(reference, os.PathSeparator)
	if err != nil {
		return nil, err
	}

	switch artType {
	case ociArtifactType:
		// TODO: should we allow custom digestAlg for OCI artifacts?
		return digestContainerImageArtifact(normalized, client)
	case digestArtifactType:
		alg, digestValue, err := parseDigestReference(normalized)
		if err != nil {
			return nil, err
		}
		return &DigestedArtifact{
			digest:    digestValue,
			digestAlg: alg,
		}, nil
	default:
		return digestLocalFileArtifact(normalized, digestAlg)
	}
}

// Digest returns the artifact's digest
func (a *DigestedArtifact) Digest() string {
	return a.digest
}

// Algorithm returns the artifact's algorithm
func (a *DigestedArtifact) Algorithm() string {
	return a.digestAlg
}

// DigestWithAlg returns the digest:algorithm of the artifact
func (a *DigestedArtifact) DigestWithAlg() string {
	return fmt.Sprintf("%s:%s", a.digestAlg, a.digest)
}

func (a *DigestedArtifact) NameRef() name.Reference {
	return a.nameRef
}
