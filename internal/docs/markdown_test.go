package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestGenMdDoc(t *testing.T) {
	linkHandler := func(s string) string { return s }

	// We generate on subcommand so we have both subcommands and parents.
	buf := new(bytes.Buffer)
	if err := genMarkdownCustom(echoCmd, buf, linkHandler); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	checkStringContains(t, output, echoCmd.Long)
	checkStringContains(t, output, echoCmd.Example)
	checkStringContains(t, output, "boolone")
	checkStringContains(t, output, "rootflag")
	checkStringOmits(t, output, rootCmd.Short)
	checkStringOmits(t, output, echoSubCmd.Short)
	checkStringOmits(t, output, deprecatedCmd.Short)
	checkStringContains(t, output, "Options inherited from parent commands")
}

func TestGenMdDocWithNoLongOrSynopsis(t *testing.T) {
	linkHandler := func(s string) string { return s }

	// We generate on subcommand so we have both subcommands and parents.
	buf := new(bytes.Buffer)
	if err := genMarkdownCustom(dummyCmd, buf, linkHandler); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	checkStringContains(t, output, dummyCmd.Example)
	checkStringContains(t, output, dummyCmd.Short)
	checkStringContains(t, output, "Options inherited from parent commands")
	checkStringOmits(t, output, "### Synopsis")
}

func TestGenMdNoHiddenParents(t *testing.T) {
	linkHandler := func(s string) string { return s }

	// We generate on subcommand so we have both subcommands and parents.
	for _, name := range []string{"rootflag", "strtwo"} {
		f := rootCmd.PersistentFlags().Lookup(name)
		f.Hidden = true
		defer func() { f.Hidden = false }()
	}
	buf := new(bytes.Buffer)
	if err := genMarkdownCustom(echoCmd, buf, linkHandler); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	checkStringContains(t, output, echoCmd.Long)
	checkStringContains(t, output, echoCmd.Example)
	checkStringContains(t, output, "boolone")
	checkStringOmits(t, output, "rootflag")
	checkStringOmits(t, output, rootCmd.Short)
	checkStringOmits(t, output, echoSubCmd.Short)
	checkStringOmits(t, output, deprecatedCmd.Short)
	checkStringOmits(t, output, "Options inherited from parent commands")
}

func TestGenMdTree(t *testing.T) {
	c := &cobra.Command{Use: "do [OPTIONS] arg1 arg2"}
	tmpdir, err := os.MkdirTemp("", "test-gen-md-tree")
	if err != nil {
		t.Fatalf("Failed to create tmpdir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	if err := GenMarkdownTreeCustom(c, tmpdir, func(s string) string { return s }, func(s string) string { return s }); err != nil {
		t.Fatalf("GenMarkdownTree failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpdir, "do.md")); err != nil {
		t.Fatalf("Expected file 'do.md' to exist")
	}
}

func BenchmarkGenMarkdownToFile(b *testing.B) {
	file, err := os.CreateTemp(b.TempDir(), "")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	linkHandler := func(s string) string { return s }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := genMarkdownCustom(rootCmd, file, linkHandler); err != nil {
			b.Fatal(err)
		}
	}
}
