// Package markdown provides terminal markdown rendering,
// with code block syntax highlighting support.
// This package is sourced from https://github.com/tj/go-termd under the terms of the MIT license.
// It was inlined to work around some dependency issues.
package markdown

import (
	"fmt"
	"strings"

	"github.com/kr/text"
	"github.com/mitchellh/go-wordwrap"
	blackfriday "github.com/russross/blackfriday/v2"
)

// Compiler is the markdown to text compiler. The zero value can be used.
type Compiler struct {
	// Columns is the number of columns to wrap text, defaulting to 90.
	Columns int

	// Markdown is an optional instance of a blackfriday markdown parser,
	// defaulting to one with CommonExtensions enabled.
	Markdown *blackfriday.Markdown

	// SyntaxHighlighter is an optional syntax highlighter for code blocks,
	// using the low-level SyntaxHighlighter interface, or SyntaxTheme map.
	SyntaxHighlighter

	inBlockQuote bool
	inList       bool
}

// Compile returns a terminal-styled plain text representation of a markdown string.
func (c *Compiler) Compile(s string) string {
	if c.Markdown == nil {
		c.Markdown = blackfriday.New(blackfriday.WithExtensions(blackfriday.CommonExtensions))
	}

	if c.Columns == 0 {
		c.Columns = 90
	}

	if c.SyntaxHighlighter == nil {
		c.SyntaxHighlighter = SyntaxTheme{}
	}

	n := c.Markdown.Parse([]byte(s))
	return c.visit(n)
}

// visit returns a compiled node string.
func (c *Compiler) visit(n *blackfriday.Node) (s string) {
	switch n.Type {
	case blackfriday.Document:
		s = c.visit(n.FirstChild)
	case blackfriday.BlockQuote:
		prev := c.inBlockQuote
		c.inBlockQuote = true
		s = c.visit(n.FirstChild)
		s = fmt.Sprintf("\033[38;5;102m%s\033[m", s)
		c.inBlockQuote = prev
	case blackfriday.List:
		prev := c.inList
		c.inList = true
		s = fmt.Sprintf("%s\n", c.visit(n.FirstChild))
		c.inList = prev
	case blackfriday.Item:
		s = fmt.Sprintf("  - %s", c.visit(n.FirstChild))
	case blackfriday.Paragraph:
		s = c.visit(n.FirstChild)
		if c.inList {
			s += "\n"
		} else {
			s = wordwrap.WrapString(s, uint(c.Columns))
			s = text.Indent(s, "  ")
			s += "\n\n"
		}
	case blackfriday.Heading:
		h := strings.Repeat("#", n.HeadingData.Level)
		t := c.visit(n.FirstChild)
		s = fmt.Sprintf("\033[1m%s %s\033[m\n\n", h, t)
	case blackfriday.HorizontalRule:
		s = fmt.Sprintf("  %s\n\n", strings.Repeat("─", c.Columns))
	case blackfriday.Emph:
		s = c.visit(n.FirstChild)
		if !c.inBlockQuote {
			s = fmt.Sprintf("\033[3m%s\033[m", s)
		}
	case blackfriday.Strong:
		s = c.visit(n.FirstChild)
		if !c.inBlockQuote {
			s = fmt.Sprintf("\033[1m%s\033[m", s)
		}
	case blackfriday.Link:
		s = c.visit(n.FirstChild)
		d := string(n.LinkData.Destination)
		if s != d {
			s = fmt.Sprintf("%s (%s)", s, d)
		}
	case blackfriday.Image:
		s = string(n.LinkData.Destination)
	case blackfriday.Text:
		s = string(n.Literal)
	case blackfriday.CodeBlock:
		lang := string(n.CodeBlockData.Info)
		s = string(n.Literal)
		s = highlight(s, lang, c.SyntaxHighlighter)
		s = fmt.Sprintf("%s\n", text.Indent(s, "    "))
	case blackfriday.Code:
		s = fmt.Sprintf("\033[38;5;102m`%s`\033[m", string(n.Literal))
	case blackfriday.HTMLSpan:
		// ignore
	default:
		s = fmt.Sprintf("<unhandled: %v>", n)
	}

	if n.Next != nil {
		s += c.visit(n.Next)
	}

	return
}
