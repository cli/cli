// This package is sourced from https://github.com/tj/go-termd under the terms of the MIT license.
// It was inlined to work around some dependency issues.
package markdown

import (
	"fmt"

	"github.com/alecthomas/chroma"
	"github.com/aybabtme/rgbterm"
	"github.com/tj/go-css/csshex"
)

// Style is the configuration used to style a particular token.
type Style struct {
	Color      string `json:"color"`
	Background string `json:"background"`
	Bold       bool   `json:"bold"`
	Faint      bool   `json:"faint"`
	Italic     bool   `json:"italic"`
	Underline  bool   `json:"underline"`
}

// apply returns a string with the style applied.
func (s Style) apply(v string) string {
	if s.Bold {
		v = escape(1, v)
	}

	if s.Faint {
		v = escape(2, v)
	}

	if s.Italic {
		v = escape(3, v)
	}

	if s.Underline {
		v = escape(4, v)
	}

	if s.Color != "" {
		v = foreground(v, s.Color)
	}

	if s.Background != "" {
		v = background(v, s.Background)
	}

	return v
}

// SyntaxTheme is a map of token names to style configurations.
type SyntaxTheme map[string]Style

// Token implementation.
func (c SyntaxTheme) Token(t chroma.Token) string {
	// specific
	if s, ok := c.mapped(t.Type, t.Value); ok {
		return s
	}

	// sub-category
	if s, ok := c.mapped(t.Type.SubCategory(), t.Value); ok {
		return s
	}

	// category
	if s, ok := c.mapped(t.Type.Category(), t.Value); ok {
		return s
	}

	return t.Value
}

// mapped returns a string mapped to its style, or returns the input string as-is.
func (c SyntaxTheme) mapped(t chroma.TokenType, v string) (string, bool) {
	// check if the key is valid
	k, ok := themeKeys[t]
	if !ok {
		return v, false
	}

	// check if the style is mapped
	s, ok := c[k]
	if !ok {
		return v, false
	}

	return s.apply(v), true
}

// escape returns an ansi escape sequence with the given code.
func escape(code int, s string) string {
	return fmt.Sprintf("\033[%dm%s\033[m", code, s)
}

// foreground color.
func foreground(s, color string) string {
	r, g, b, ok := csshex.Parse(color)
	if !ok {
		return s
	}
	return rgbterm.FgString(s, r, g, b)
}

// background color.
func background(s, color string) string {
	r, g, b, ok := csshex.Parse(color)
	if !ok {
		return s
	}
	return rgbterm.BgString(s, r, g, b)
}

// themeKeys is the map of token types to names.
var themeKeys = map[chroma.TokenType]string{
	chroma.Keyword:                  "keyword",
	chroma.KeywordConstant:          "keyword.constant",
	chroma.KeywordDeclaration:       "keyword.declaration",
	chroma.KeywordNamespace:         "keyword.namespace",
	chroma.KeywordPseudo:            "keyword.pseudo",
	chroma.KeywordReserved:          "keyword.reserved",
	chroma.KeywordType:              "keyword.type",
	chroma.Name:                     "name",
	chroma.NameAttribute:            "name.attribute",
	chroma.NameBuiltin:              "name.builtin",
	chroma.NameBuiltinPseudo:        "name.builtin.pseudo",
	chroma.NameClass:                "name.class",
	chroma.NameConstant:             "name.constant",
	chroma.NameDecorator:            "name.decorator",
	chroma.NameEntity:               "name.entity",
	chroma.NameException:            "name.exception",
	chroma.NameFunction:             "name.function",
	chroma.NameFunctionMagic:        "name.function.magic",
	chroma.NameKeyword:              "name.keyword",
	chroma.NameLabel:                "name.label",
	chroma.NameNamespace:            "name.namespace",
	chroma.NameOperator:             "name.operator",
	chroma.NameOther:                "name.other",
	chroma.NamePseudo:               "name.pseudo",
	chroma.NameProperty:             "name.property",
	chroma.NameTag:                  "name.tag",
	chroma.NameVariable:             "name.variable",
	chroma.NameVariableAnonymous:    "name.variable.anonymous",
	chroma.NameVariableClass:        "name.variable.class",
	chroma.NameVariableGlobal:       "name.variable.global",
	chroma.NameVariableInstance:     "name.variable.instance",
	chroma.NameVariableMagic:        "name.variable.magic",
	chroma.Literal:                  "literal",
	chroma.LiteralDate:              "literal.date",
	chroma.LiteralOther:             "literal.other",
	chroma.LiteralString:            "literal.string",
	chroma.LiteralStringAffix:       "literal.string.affix",
	chroma.LiteralStringAtom:        "literal.string.atom",
	chroma.LiteralStringBacktick:    "literal.string.backtick",
	chroma.LiteralStringBoolean:     "literal.string.boolean",
	chroma.LiteralStringChar:        "literal.string.char",
	chroma.LiteralStringDelimiter:   "literal.string.delimiter",
	chroma.LiteralStringDoc:         "literal.string.doc",
	chroma.LiteralStringDouble:      "literal.string.double",
	chroma.LiteralStringEscape:      "literal.string.escape",
	chroma.LiteralStringHeredoc:     "literal.string.heredoc",
	chroma.LiteralStringInterpol:    "literal.string.interpol",
	chroma.LiteralStringName:        "literal.string.name",
	chroma.LiteralStringOther:       "literal.string.other",
	chroma.LiteralStringRegex:       "literal.string.regex",
	chroma.LiteralStringSingle:      "literal.string.single",
	chroma.LiteralStringSymbol:      "literal.string.symbol",
	chroma.LiteralNumber:            "literal.number",
	chroma.LiteralNumberBin:         "literal.number.bin",
	chroma.LiteralNumberFloat:       "literal.number.float",
	chroma.LiteralNumberHex:         "literal.number.hex",
	chroma.LiteralNumberInteger:     "literal.number.integer",
	chroma.LiteralNumberIntegerLong: "literal.number.integer.long",
	chroma.LiteralNumberOct:         "literal.number.oct",
	chroma.Operator:                 "operator",
	chroma.OperatorWord:             "operator.word",
	chroma.Punctuation:              "punctuation",
	chroma.Comment:                  "comment",
	chroma.CommentHashbang:          "comment.hashbang",
	chroma.CommentMultiline:         "comment.multiline",
	chroma.CommentSingle:            "comment.single",
	chroma.CommentSpecial:           "comment.special",
	chroma.CommentPreproc:           "comment.preproc",
	chroma.CommentPreprocFile:       "comment.preproc.file",
	chroma.Generic:                  "generic",
	chroma.GenericDeleted:           "generic.deleted",
	chroma.GenericEmph:              "generic.emph",
	chroma.GenericError:             "generic.error",
	chroma.GenericHeading:           "generic.heading",
	chroma.GenericInserted:          "generic.inserted",
	chroma.GenericOutput:            "generic.output",
	chroma.GenericPrompt:            "generic.prompt",
	chroma.GenericStrong:            "generic.strong",
	chroma.GenericSubheading:        "generic.subheading",
	chroma.GenericTraceback:         "generic.traceback",
	chroma.GenericUnderline:         "generic.underline",
	chroma.Text:                     "text",
	chroma.TextWhitespace:           "text.whitespace",
	chroma.TextSymbol:               "text.symbol",
	chroma.TextPunctuation:          "text.punctuation",
}
