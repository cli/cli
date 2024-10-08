package acceptance_test

import (
	"os"
	"path"
	"strings"
	"testing"

	"math/rand"

	"github.com/cli/cli/v2/internal/ghcmd"
	"github.com/rogpeppe/go-internal/testscript"
)

func ghMain() int {
	return int(ghcmd.Main())
}

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"gh": ghMain,
	}))
}

func TestPullRequests(t *testing.T) {
	testscript.Run(t, params("pr"))
}

func params(dir string) testscript.Params {
	return testscript.Params{
		Dir:                 path.Join("testdata", dir),
		Files:               []string{},
		Setup:               sharedSetup,
		Cmds:                sharedCmds,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
	}
}

var sharedSetup = func(ts *testscript.Env) error {
	scriptName, ok := extractScriptName(ts.Vars)
	if !ok {
		ts.T().Fatal("script name not found")
	}
	ts.Setenv("SCRIPT_NAME", scriptName)

	ts.Setenv("HOME", ts.Cd)
	ts.Setenv("GH_CONFIG_DIR", ts.Cd)

	ts.Setenv("GH_TOKEN", os.Getenv("GH_TOKEN"))

	ts.Setenv("ORG", os.Getenv("GH_ACCEPTANCE_ORG"))

	ts.Setenv("RANDOM_STRING", randomString(10))
	return nil
}

var sharedCmds = map[string]func(ts *testscript.TestScript, neg bool, args []string){
	"defer": func(ts *testscript.TestScript, neg bool, args []string) {
		ts.Defer(func() {
			ts.Check(ts.Exec(args[0], args[1:]...))
		})
	},
	"stdout2env": func(ts *testscript.TestScript, neg bool, args []string) {
		if neg {
			ts.Fatalf("unsupported: ! stdout2env")
		}
		if len(args) != 1 {
			ts.Fatalf("usage: stdout2env name")
		}

		ts.Setenv(args[0], strings.TrimRight(ts.ReadFile("stdout"), "\n"))
	},
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func extractScriptName(vars []string) (string, bool) {
	for _, kv := range vars {
		if strings.HasPrefix(kv, "WORK=") {
			v := strings.Split(kv, "=")[1]
			return strings.CutPrefix(path.Base(v), "script-")
		}
	}
	return "", false
}
