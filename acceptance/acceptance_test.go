//go:build acceptance

package acceptance_test

import (
	"fmt"
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
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	testscript.Run(t, testScriptParamsFor("pr"))
}

func testScriptParamsFor(dir string) testscript.Params {
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

	ts.Setenv("GH_HOST", os.Getenv("GH_HOST"))

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

type missingEnvError struct {
	missingEnvs []string
}

func (e missingEnvError) Error() string {
	return fmt.Sprintf("missing environment variables: %s", strings.Join(e.missingEnvs, ", "))
}

type testScriptEnv struct {
	host  string
	org   string
	token string
}

func (e *testScriptEnv) fromEnv() error {
	envMap := map[string]string{}

	var missingEnvs []string
	for _, key := range []string{"GH_HOST", "GH_ACCEPTANCE_ORG", "GH_TOKEN"} {
		val, ok := os.LookupEnv(key)
		if !ok {
			missingEnvs = append(missingEnvs, key)
			continue
		}

		envMap[key] = val
	}

	if len(missingEnvs) > 0 {
		return missingEnvError{missingEnvs: missingEnvs}
	}

	e.host = envMap["GH_HOST"]
	e.org = envMap["GH_ACCEPTANCE_ORG"]
	e.token = envMap["GH_TOKEN"]

	return nil
}
