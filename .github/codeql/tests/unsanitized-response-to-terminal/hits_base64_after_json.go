package fixtures

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A base64 field decoded back into raw bytes, then printed to Out. The decode
// reintroduces content that escaped any sanitization of the encoded text. Must
// be flagged.
type blobResponse struct {
	Content string
}

func fetchBlob(resp blobResponse) (string, error) {
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(resp.Content)))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func PreviewBlob(resp blobResponse, ios *iostreams.IOStreams) error {
	content, err := fetchBlob(resp)
	if err != nil {
		return err
	}
	fmt.Fprint(ios.Out, content)
	return nil
}
