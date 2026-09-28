package root

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestDedent(t *testing.T) {
	type c struct {
		input    string
		expected string
	}

	cases := []c{
		{
			input:    "      --help      Show help for command\n      --version   Show gh version\n",
			expected: "--help      Show help for command\n--version   Show gh version\n",
		},
		{
			input:    "      --help              Show help for command\n  -R, --repo OWNER/REPO   Select another repository using the OWNER/REPO format\n",
			expected: "    --help              Show help for command\n-R, --repo OWNER/REPO   Select another repository using the OWNER/REPO format\n",
		},
		{
			input:    "  line 1\n\n  line 2\n line 3",
			expected: " line 1\n\n line 2\nline 3",
		},
		{
			input:    "  line 1\n  line 2\n  line 3\n\n",
			expected: "line 1\nline 2\nline 3\n\n",
		},
		{
			input:    "\n\n\n\n\n\n",
			expected: "\n\n\n\n\n\n",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tt := range cases {
		got := dedent(tt.input)
		if got != tt.expected {
			t.Errorf("expected: %q, got: %q", tt.expected, got)
		}
	}
}

// Since our online docs website renders pages by using the kramdown (a superset
// of Markdown) engine, we have to check against some known quirks of the
// syntax.
func TestKramdownCompatibleDocs(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
		Browser:   &browser.Stub{},
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func() []extensions.Extension {
				return nil
			},
		},
	}

	cmd, err := NewCmdRoot(f, &telemetry.NoOpService{}, "N/A", "")
	require.NoError(t, err)

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		name := fmt.Sprintf("%q: test pipes are in code blocks", cmd.UseLine())
		t.Run(name, func(t *testing.T) {
			assertPipesAreInCodeBlocks(t, cmd)
		})
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}

	walk(cmd)
}

// If not in a code block or a code span, kramdown treats pipes ("|") as table
// column separators, even if there's no table header, or left/right table row
// borders (i.e. lines starting and ending with a pipe).
//
// We need to assert there's no pipe in the text unless it's in a code-block or
// code-span.
//
// (See https://github.com/cli/cli/issues/10348)
func assertPipesAreInCodeBlocks(t *testing.T, cmd *cobra.Command) {
	md := goldmark.New()
	reader := text.NewReader([]byte(cmd.Long))
	doc := md.Parser().Parse(reader)

	var checkNode func(node ast.Node)
	checkNode = func(node ast.Node) {
		if node.Kind() == ast.KindCodeSpan || node.Kind() == ast.KindCodeBlock {
			return
		}

		if node.Kind() == ast.KindText {
			text := string(node.(*ast.Text).Segment.Value(reader.Source()))
			require.NotContains(t, text, "|", `found pipe ("|") in plain text in %q docs`, cmd.CommandPath())
		}

		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			checkNode(child)
		}
	}

	checkNode(doc)
}

func TestWriteHelp_aliasResolution(t *testing.T) {
	newRoot := func() (*cobra.Command, *iostreams.IOStreams) {
		ios, _, _, _ := iostreams.Test()
		rootCmd := &cobra.Command{Use: "gh"}
		prCmd := &cobra.Command{Use: "pr", Short: "Manage pull requests"}
		listCmd := &cobra.Command{Use: "list", Short: "List pull requests", Run: func(*cobra.Command, []string) {}}
		listCmd.Flags().Int("limit", 30, "Maximum number of items to fetch")
		prCmd.AddCommand(listCmd)
		rootCmd.AddCommand(prCmd)
		return rootCmd, ios
	}

	t.Run("configured alias renders the target command help", func(t *testing.T) {
		rootCmd, ios := newRoot()
		aliasCmd := NewCmdAlias(ios, "prs", "pr list")
		rootCmd.AddCommand(aliasCmd)

		var buf bytes.Buffer
		WriteHelp(&buf, ios.ColorScheme(), aliasCmd)

		out := buf.String()
		require.Contains(t, out, "gh pr list")
		require.Contains(t, out, "--limit")
		require.NotContains(t, out, `Alias for "pr list"`)
	})

	t.Run("alias expansion carrying flags still resolves to the target", func(t *testing.T) {
		rootCmd, ios := newRoot()
		aliasCmd := NewCmdAlias(ios, "prs", "pr list --limit 5")
		rootCmd.AddCommand(aliasCmd)

		var buf bytes.Buffer
		WriteHelp(&buf, ios.ColorScheme(), aliasCmd)

		require.Contains(t, buf.String(), "gh pr list")
	})

	t.Run("shell alias has no target and renders its own help", func(t *testing.T) {
		rootCmd, ios := newRoot()
		aliasCmd := NewCmdShellAlias(ios, "sh", "!echo hi")
		rootCmd.AddCommand(aliasCmd)

		var buf bytes.Buffer
		WriteHelp(&buf, ios.ColorScheme(), aliasCmd)

		require.Contains(t, buf.String(), "gh sh")
	})

	t.Run("alias expanding to an unknown command falls back to the alias", func(t *testing.T) {
		rootCmd, ios := newRoot()
		aliasCmd := NewCmdAlias(ios, "zz", "nope missing")
		rootCmd.AddCommand(aliasCmd)

		var buf bytes.Buffer
		WriteHelp(&buf, ios.ColorScheme(), aliasCmd)

		require.Contains(t, buf.String(), "gh zz")
	})

	t.Run("non-alias command is unaffected", func(t *testing.T) {
		rootCmd, ios := newRoot()
		target, _, err := rootCmd.Find([]string{"pr", "list"})
		require.NoError(t, err)

		var buf bytes.Buffer
		WriteHelp(&buf, ios.ColorScheme(), target)

		require.Contains(t, buf.String(), "gh pr list")
	})
}
