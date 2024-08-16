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

func TestGenManAliases(t *testing.T) {
	buf := new(bytes.Buffer)
	header := &GenManHeader{}
	if err := renderMan(aliasCmd, header, buf); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	checkStringContains(t, output, translate(aliasCmd.Name()))
	checkStringContains(t, output, "ALIASES")
	checkStringContains(t, output, "foo")
	checkStringContains(t, output, "yoo")
}

func TestGenManJSONFields(t *testing.T) {
	buf := new(bytes.Buffer)
	header := &GenManHeader{}
	if err := renderMan(jsonCmd, header, buf); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	checkStringContains(t, output, translate(jsonCmd.Name()))
	checkStringContains(t, output, "JSON FIELDS")
	checkStringContains(t, output, "foo")
	checkStringContains(t, output, "bar")
	checkStringContains(t, output, "baz")
}

func TestGenManDocExitCodes(t *testing.T) {
	header := &GenManHeader{
		Title:   "Project",
		Section: "1",
	}
	cmd := &cobra.Command{
		Use:   "test-command",
		Short: "A test command",
		Long:  "A test command for checking exit codes section",
	}
	buf := new(bytes.Buffer)
	if err := renderMan(cmd, header, buf); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	// Check for the presence of the exit codes section
	checkStringContains(t, output, ".SH EXIT CODES")
	checkStringContains(t, output, "0: Successful execution")
	checkStringContains(t, output, "1: Error")
	checkStringContains(t, output, "2: Command canceled")
	checkStringContains(t, output, "4: Authentication required")
	checkStringContains(t, output, "NOTE: Specific commands may have additional exit codes. Refer to the command's help for more information.")
}

func TestManPrintFlagsHidesShortDeprecated(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().StringP("foo", "f", "default", "Foo flag")
	_ = c.Flags().MarkShorthandDeprecated("foo", "don't use it no more")

	buf := new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got := buf.String()
	expected := "`--foo` `<string> (default \"default\")`\n:   Foo flag\n\n"
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

func TestManPrintFlagsShowsDefaultValues(t *testing.T) {
	type TestOptions struct {
		Limit     int
		Template  string
		Fork      bool
		NoArchive bool
		Topic     []string
	}
	opts := TestOptions{}
	// Int flag should show it
	c := &cobra.Command{}
	c.Flags().IntVar(&opts.Limit, "limit", 30, "Some limit")

	buf := new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got := buf.String()
	expected := "`--limit` `<int> (default 30)`\n:   Some limit\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// Bool flag should hide it if default is false
	c = &cobra.Command{}
	c.Flags().BoolVar(&opts.Fork, "fork", false, "Show only forks")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--fork`\n:   Show only forks\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// Bool flag should show it if default is true
	c = &cobra.Command{}
	c.Flags().BoolVar(&opts.NoArchive, "no-archived", true, "Hide archived")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--no-archived` `(default true)`\n:   Hide archived\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// String flag should show it if default is not an empty string
	c = &cobra.Command{}
	c.Flags().StringVar(&opts.Template, "template", "T1", "Some template")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--template` `<string> (default \"T1\")`\n:   Some template\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// String flag should hide it if default is an empty string
	c = &cobra.Command{}
	c.Flags().StringVar(&opts.Template, "template", "", "Some template")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--template` `<string>`\n:   Some template\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// String slice flag should hide it if default is an empty slice
	c = &cobra.Command{}
	c.Flags().StringSliceVar(&opts.Topic, "topic", nil, "Some topics")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--topic` `<strings>`\n:   Some topics\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}

	// String slice flag should show it if default is not an empty slice
	c = &cobra.Command{}
	c.Flags().StringSliceVar(&opts.Topic, "topic", []string{"apples", "oranges"}, "Some topics")

	buf = new(bytes.Buffer)
	manPrintFlags(buf, c.Flags())

	got = buf.String()
	expected = "`--topic` `<strings> (default [apples,oranges])`\n:   Some topics\n\n"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
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
