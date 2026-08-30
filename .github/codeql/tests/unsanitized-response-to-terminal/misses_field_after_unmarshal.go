package fixtures

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// A helper reads the raw body into bytes; the caller json.Unmarshals it and
// prints a decoded field. Statically this is identical to the safe case where an
// already-sanitized JSON response is decoded and a field printed, so the query
// stays silent on purpose. The runtime ContentOut writer is the mitigation. Must
// NOT be flagged.
type logEntry struct {
	Content string
}

func fetchLogBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func RenderLogField(url string, ios *iostreams.IOStreams) error {
	raw, err := fetchLogBytes(url)
	if err != nil {
		return err
	}
	var entry logEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return err
	}
	fmt.Fprintln(ios.Out, entry.Content)
	return nil
}
