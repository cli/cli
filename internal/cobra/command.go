package cobra

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

type Command struct {
	Use     string
	Short   string
	Long    string
	Example string

	Hidden      bool
	Deprecated  string
	Annotations map[string]string
	Aliases     []string
	SuggestFor  []string

	DisableFlagParsing         bool
	DisableFlagsInUseLine      bool
	SilenceErrors              bool
	SilenceUsage               bool
	SuggestionsMinimumDistance int

	PreRun  func(cmd *Command, args []string)
	Run     func(cmd *Command, args []string)
	PreRunE func(cmd *Command, args []string) error
	RunE    func(cmd *Command, args []string) error
	Args    PositionalArgs

	PersistentPreRunE func(cmd *Command, args []string) error
	ValidArgsFunction func(cmd *Command, args []string, toComplete string) ([]string, ShellCompDirective)

	args          []string
	outStream     io.Writer
	errStream     io.Writer
	localFlags    *pflag.FlagSet
	parentCommand *Command
	childCommands []*Command
}

func (c *Command) SetArgs(args []string) {
	c.args = args
}

func (c *Command) SetIn(io.Reader) {}
func (c *Command) SetOut(w io.Writer) {
	c.outStream = w
}
func (c *Command) SetErr(w io.Writer) {
	c.errStream = w
}

func (c *Command) ErrOrStderr() io.Writer {
	if c.errStream != nil {
		return c.errStream
	}
	return os.Stderr
}

func (c *Command) Context() context.Context {
	return nil
}

func (c *Command) Name() string {
	if idx := strings.IndexRune(c.Use, ' '); idx != -1 {
		return c.Use[:idx]
	}
	return c.Use
}

func (c *Command) UseLine() string {
	return c.Use
}
