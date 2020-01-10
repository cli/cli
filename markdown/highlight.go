// This package is sourced from https://github.com/tj/go-termd under the terms of the MIT license.
// It was inlined to work around some dependency issues.
package markdown

import (
	"strings"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/lexers"
)

// SyntaxHighlighter is the interface used to highlight blocks of code.
type SyntaxHighlighter interface {
	Token(chroma.Token) string
}

// highlight returns highlighted code, or the input text on error.
func highlight(source, lang string, highlight SyntaxHighlighter) string {
	l := lexers.Get(lang)
	if l == nil {
		return source
	}

	l = chroma.Coalesce(l)

	it, err := l.Tokenise(nil, source)
	if err != nil {
		return source
	}

	var w strings.Builder
	for _, t := range it.Tokens() {
		w.WriteString(highlight.Token(t))
	}
	return w.String()
}
