package artifact

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
)

type artifactType int

const (
	ociArtifactType artifactType = iota
	fileArtifactType
)

// DigestedArtifact abstracts the software artifact being verified
type DigestedArtifact struct {
	URL       string
	digest    string
	digestAlg string
	nameRef   name.Reference
}

func normalizeReference(reference string, pathSeparator rune) (normalized string, artifactType artifactType, err error) {
	switch {
	case strings.HasPrefix(reference, "oci://"):
		return reference[6:], ociArtifactType, nil
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

func NewDigestedArtifact(client oci.Client, reference, digestAlg string) (artifact *DigestedArtifact, err error) {
	normalized, artifactType, err := normalizeReference(reference, os.PathSeparator)
	if err != nil {
		return nil, err
	}
	if artifactType == ociArtifactType {
		// TODO: should we allow custom digestAlg for OCI artifacts?
		return digestContainerImageArtifact(normalized, client)
	}
	return digestLocalFileArtifact(normalized, digestAlg)
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
