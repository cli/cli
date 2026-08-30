// Minimal stub of github.com/cli/go-gh/v2/pkg/asciisanitizer for CodeQL test
// extraction. Only needs the Sanitizer type to exist under the expected
// qualified name so the barrier predicate's hasQualifiedName check matches.
package asciisanitizer

type Sanitizer struct{}

func (s *Sanitizer) Reset() {}

func (s *Sanitizer) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	return 0, 0, nil
}
