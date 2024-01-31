// jsonmerge implements readers to merge JSON arrays or objects.
package jsonmerge

import (
	"io"
)

// Merger is implemented to merge JSON arrays or objects.
type Merger interface {
	NewPage(r io.Reader, isLastPage bool) io.ReadCloser
	Close() error
}
