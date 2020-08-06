package command

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/utils"
	"github.com/google/shlex"
	"github.com/spf13/pflag"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

const defaultTestConfig = `hosts:
  github.com:
    user: OWNER
    oauth_token: "1234567890"
`

func initBlankContext(cfg, repo, branch string) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo(repo)
		ctx.SetBranch(branch)
		ctx.SetRemotes(map[string]string{
			"origin": "OWNER/REPO",
		})

		if cfg == "" {
			cfg = defaultTestConfig
		}

		// NOTE we are not restoring the original readConfig; we never want to touch the config file on
		// disk during tests.
		config.StubConfig(cfg, "")

		return ctx
	}
}

type cmdOut struct {
	outBuf, errBuf *bytes.Buffer
}

func (c cmdOut) String() string {
	return c.outBuf.String()
}

func (c cmdOut) Stderr() string {
	return c.errBuf.String()
}

func RunCommand(args string) (*cmdOut, error) {
	rootCmd := RootCmd
	rootArgv, err := shlex.Split(args)
	if err != nil {
		return nil, err
	}

	cmd, _, err := rootCmd.Traverse(rootArgv)
	if err != nil {
		return nil, err
	}

	rootCmd.SetArgs(rootArgv)

	outBuf := bytes.Buffer{}
	cmd.SetOut(&outBuf)
	errBuf := bytes.Buffer{}
	cmd.SetErr(&errBuf)

	// Reset flag values so they don't leak between tests
	// FIXME: change how we initialize Cobra commands to render this hack unnecessary
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		switch v := f.Value.(type) {
		case pflag.SliceValue:
			_ = v.Replace([]string{})
		default:
			switch v.Type() {
			case "bool", "string", "int":
				_ = v.Set(f.DefValue)
			}
		}
	})

	_, err = rootCmd.ExecuteC()
	cmd.SetOut(nil)
	cmd.SetErr(nil)

	return &cmdOut{&outBuf, &errBuf}, err
}

func stubTerminal(connected bool) func() {
	isTerminal := utils.IsTerminal
	utils.IsTerminal = func(_ interface{}) bool {
		return connected
	}

	terminalSize := utils.TerminalSize
	if connected {
		utils.TerminalSize = func(_ interface{}) (int, int, error) {
			return 80, 20, nil
		}
	} else {
		utils.TerminalSize = func(_ interface{}) (int, int, error) {
			return 0, 0, fmt.Errorf("terminal connection stubbed to false")
		}
	}

	return func() {
		utils.IsTerminal = isTerminal
		utils.TerminalSize = terminalSize
	}
}
