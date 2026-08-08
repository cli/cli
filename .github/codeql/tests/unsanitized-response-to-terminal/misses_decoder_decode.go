package fixtures

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// json.NewDecoder on the body. The transport JSON sanitizer feeds this path, so
// the Decode barrier keeps the query silent. Must NOT be flagged.
type issueView struct {
	Title string
}

func ViewIssueTitle(url string, ios *iostreams.IOStreams) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var iss issueView
	if err := json.NewDecoder(resp.Body).Decode(&iss); err != nil {
		return err
	}
	fmt.Fprintln(ios.Out, iss.Title)
	return nil
}
