package docs

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func translate(in string) string {
	return strings.Replace(in, "-", "\\-", -1)
}

func TestGenManDoc(t *testing.T) {
	header := &GenManHeader{
		Title:   "Project",
		Section: "1",
	}

	// We generate on a subcommand so we have both subcommands and parents
	buf := new(bytes.Buffer)
	if err := renderMan(echoCmd, header, buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	// Make sure parent has - in CommandPath() in SEE ALSO:
	parentPath := echoCmd.Parent().CommandPath()
	dashParentPath := strings.Replace(parentPath, " ", "-", -1)
	expected := translate(dashParentPath)
	expected = expected + "(" + header.Section + ")"
	checkStringContains(t, output, expected)

	checkStringContains(t, output, translate(echoCmd.Name()))
	checkStringContains(t, output, translate(echoCmd.Name()))
	checkStringContains(t, output, "boolone")
	checkStringContains(t, output, "rootflag")
	checkStringContains(t, output, translate(rootCmd.Name()))
	checkStringContains(t, output, translate(echoSubCmd.Name()))
	checkStringOmits(t, output, translate(deprecatedCmd.Name()))
}

func TestGenManNoHiddenParents(t *testing.T) {
	header := &GenManHeader{
		Title:   "Project",
		Section: "1",
	}

	// We generate on a subcommand so we have both subcommands and parents
	for _, name := range []string{"rootflag", "strtwo"} {
		f := rootCmd.PersistentFlags().Lookup(name)
		f.Hidden = true
		defer func() { f.Hidden = false }()
	}
	buf := new(bytes.Buffer)
	if err := renderMan(echoCmd, header, buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	// Make sure parent has - in CommandPath() in SEE ALSO:
	parentPath := echoCmd.Parent().CommandPath()
	dashParentPath := strings.Replace(parentPath, " ", "-", -1)
	expected := translate(dashParentPath)
	expected = expected + "(" + header.Section + ")"
	checkStringContains(t, output, expected)

	checkStringContains(t, output, translate(echoCmd.Name()))
	checkStringContains(t, output, translate(echoCmd.Name()))
	checkStringContains(t, output, "boolone")
	checkStringOmits(t, output, "rootflag")
	checkStringContains(t, output, translate(rootCmd.Name()))
	checkStringContains(t, output, translate(echoSubCmd.Name()))
	checkStringOmits(t, output, translate(deprecatedCmd.Name()))
	checkStringOmits(t, output, "OPTIONS INHERITED FROM PARENT COMMANDS")
}

func TestGenManSeeAlso(t *testing.T) {
	rootCmd := &cobra.Command{Use: "root", Run: emptyRun}
	aCmd := &cobra.Command{Use: "aaa", Run: emptyRun, Hidden: true} // #229
	bCmd := &cobra.Command{Use: "bbb", Run: emptyRun}
	cCmd := &cobra.Command{Use: "ccc", Run: emptyRun}
	rootCmd.AddCommand(aCmd, bCmd, cCmd)

	buf := new(bytes.Buffer)
	header := &GenManHeader{}
	if err := renderMan(rootCmd, header, buf); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(buf)
	if err := assertLineFound(scanner, ".SH SEE ALSO"); err == nil {
		t.Fatalf("Did not expect SEE ALSO section header")
	}
}

func TestManPrintFlagsHidesShortDeprecated(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().StringP("foo", "f", "default", "Foo flag")
	_ = c.Flags().MarkShorthandDeprecated("foo", "don't use it no more")

	buf := new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got := buf.String()
	expected := "`--foo` `<string>`\n:   Foo flag\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

func TestGenManTree(t *testing.T) {
	c := &cobra.Command{Use: "do [OPTIONS] arg1 arg2"}
	tmpdir, err := os.MkdirTemp("", "test-gen-man-tree")
	if err != nil {
		t.Fatalf("Failed to create tmpdir: %s", err.Error())
	}
	defer os.RemoveAll(tmpdir)

	if err := GenManTree(c, tmpdir); err != nil {
		t.Fatalf("GenManTree failed: %s", err.Error())
	}

	if _, err := os.Stat(filepath.Join(tmpdir, "do.1")); err != nil {
		t.Fatalf("Expected file 'do.1' to exist")
	}
}

func assertLineFound(scanner *bufio.Scanner, expectedLine string) error {
	for scanner.Scan() {
		line := scanner.Text()
		if line == expectedLine {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan failed: %s", err)
	}

	return fmt.Errorf("hit EOF before finding %v", expectedLine)
}

func BenchmarkGenManToFile(b *testing.B) {
	file, err := os.CreateTemp(b.TempDir(), "")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := renderMan(rootCmd, nil, file); err != nil {
			b.Fatal(err)
		}
	}
}
