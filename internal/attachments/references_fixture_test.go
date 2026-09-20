package attachments

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateMDFixture = flag.Bool("update-md-fixture", false, "rewrite the expected output in testdata")

// fixtureAttachmentArgs is the set of attached files testdata/references_input.md is
// written against. Every asset URL is recognisable on sight so the expected
// output stays readable, and the paths cover the spellings markdown allows.
func fixtureAttachmentArgs() []attachmentArg {
	img := func(path, name string) attachmentArg {
		return attachmentArg{Path: path, URL: "https://example.com/" + name, Alt: name}
	}
	return []attachmentArg{
		img("login.png", "login"),
		{Path: "./repro.mp4", URL: "https://example.com/repro", Alt: "repro.mp4", RendersAsPlayer: true},
		img("./Screenshot 2026-08-10 at 5.38.10 PM.png", "screenshot"),
		img("./f(1).png", "parens"),
		img("./f((1)(2)).png", "nested-parens"),
		img("./f).png", "escaped-paren"),
		img("./f", "truncated"),
		img(`./a\b.png`, "backslash"),
		img("./a>b.png", "escaped-angle"),
		img("./login(1).png", "escaped-parens"),
		img("./f(1.png", "unbalanced-open"),
		img("./my file.png", "bare-space"),
		img("./unused.png", "unused"),
		// An upload that produced no URL. Its reference is left as written and
		// is not reported for appending, since there is nothing to append.
		{Path: "./nourl.png", Alt: "nourl"},
	}
}

// One markdown document covering every syntax this package handles, so the
// behaviour can be read as markdown rather than as Go string literals.
//
// Run with -update-md-fixture to rewrite the expected output, then read the diff:
// that is the review, since output regenerated from the code under test agrees
// with that code by construction.
func TestAttachAssetsToMarkdownFixture(t *testing.T) {
	const (
		input    = "testdata/references_input.md"
		expected = "testdata/references_expected.md"
	)

	markdown, err := os.ReadFile(input)
	require.NoError(t, err)

	attachmentArgs := fixtureAttachmentArgs()
	v, err := newAttachableMarkdown(string(markdown), attachmentArgs)
	require.NoError(t, err)

	got, err := attachAssetsToMarkdown(v)
	require.NoError(t, err)

	if *updateMDFixture {
		require.NoError(t, os.WriteFile(filepath.Clean(expected), []byte(got.Rewritten), 0o600))
	}

	want, err := os.ReadFile(expected)
	require.NoError(t, err, "run go test -update-md-fixture to create it")
	require.Equal(t, string(want), got.Rewritten)

	// Asserted here rather than in the expected output, since appending them
	// is the caller's job.
	var unreferenced []string
	for _, r := range got.ToAppend {
		unreferenced = append(unreferenced, r.Path)
	}
	// In the order the arguments were passed, which is the contract.
	require.Equal(t, []string{
		"./f(1.png",     // an unbalanced "(" does not parse as a destination
		"./my file.png", // a bare space does not parse as a destination
		"./unused.png",  // defined but never used, so nothing renders it
	}, unreferenced)
}
