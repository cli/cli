package githubv3

import (
	"net/http"
	"regexp"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

// Response is an HTTP response from the GitHub REST API.
type Response struct {
	*http.Response
}

// NextPage returns the rel="next" URL from the Link header, or "" if absent.
//
// It is the opaque URL the server supplied rather than a parsed page number,
// so cursor-based pagination keeps working.
func (r *Response) NextPage() string {
	var next string
	for _, m := range linkRE.FindAllStringSubmatch(r.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			next = m[1]
		}
	}
	return next
}
