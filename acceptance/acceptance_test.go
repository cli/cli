//go:build acceptance

package acceptance_test

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

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
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "pr"))
}

func TestIssues(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "pr"))
}

func TestWorkflows(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "workflow"))
}

func TestAPI(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "api"))
}

func TestAuth(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "auth"))
}

func TestReleases(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(tsEnv, "release"))
}

func TestRepo(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testScriptParamsFor(tsEnv, "repo"))
}

func testScriptParamsFor(tsEnv testScriptEnv, command string) testscript.Params {
	var files []string
	if tsEnv.script != "" {
		files = []string{path.Join("testdata", command, tsEnv.script)}
	}

	var dir string
	if len(files) == 0 {
		dir = path.Join("testdata", command)
	}

	return testscript.Params{
		Dir:                 dir,
		Files:               files,
		Setup:               sharedSetup(tsEnv),
		Cmds:                sharedCmds(tsEnv),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		TestWork:            tsEnv.preserveWorkDir,
	}
}

var keyT struct{}

func sharedSetup(tsEnv testScriptEnv) func(ts *testscript.Env) error {
	return func(ts *testscript.Env) error {
		scriptName, ok := extractScriptName(ts.Vars)
		if !ok {
			ts.T().Fatal("script name not found")
		}
		ts.Setenv("SCRIPT_NAME", scriptName)
		ts.Setenv("HOME", ts.Cd)
		ts.Setenv("GH_CONFIG_DIR", ts.Cd)

		ts.Setenv("GH_HOST", tsEnv.host)
		ts.Setenv("ORG", tsEnv.org)
		ts.Setenv("GH_TOKEN", tsEnv.token)

		ts.Setenv("RANDOM_STRING", randomString(10))

		ts.Values[keyT] = ts.T()
		return nil
	}
}

// sharedCmds defines a collection of custom testscript commands for our use.
func sharedCmds(tsEnv testScriptEnv) map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"defer": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! defer")
			}

			if tsEnv.skipDefer {
				return
			}

			tt, ok := ts.Value(keyT).(testscript.T)
			if !ok {
				ts.Fatalf("%v is not a testscript.T", ts.Value(keyT))
			}

			ts.Defer(func() {
				// If you're wondering why we're not using ts.Check here, it's because it raises a panic, and testscript
				// only catches the panics directly from commands, not from the deferred functions. So what we do
				// instead is grab the `t` in the setup function and store it as a value. It's important that we use
				// `t` from the setup function because it represents the subtest created for each individual script,
				// rather than each top-level test.
				// See: https://github.com/rogpeppe/go-internal/issues/276
				if err := ts.Exec(args[0], args[1:]...); err != nil {
					tt.FailNow()
				}
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
		"sleep": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! sleep")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: sleep seconds")
			}

			// sleep for the given number of seconds
			seconds, err := strconv.Atoi(args[0])
			if err != nil {
				ts.Fatalf("invalid number of seconds: %v", err)
			}

			d := time.Duration(seconds) * time.Second
			time.Sleep(d)
		},
	}
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
	return fmt.Sprintf("environment variable(s) %s must be set and non-empty", strings.Join(e.missingEnvs, ", "))
}

type testScriptEnv struct {
	host  string
	org   string
	token string

	script string

	skipDefer       bool
	preserveWorkDir bool
}

func (e *testScriptEnv) fromEnv() error {
	envMap := map[string]string{}

	requiredEnvVars := []string{
		"GH_ACCEPTANCE_HOST",
		"GH_ACCEPTANCE_ORG",
		"GH_ACCEPTANCE_TOKEN",
	}

	var missingEnvs []string
	for _, key := range requiredEnvVars {
		val, ok := os.LookupEnv(key)
		if val == "" || !ok {
			missingEnvs = append(missingEnvs, key)
			continue
		}

		envMap[key] = val
	}

	if len(missingEnvs) > 0 {
		return missingEnvError{missingEnvs: missingEnvs}
	}

	if envMap["GH_ACCEPTANCE_ORG"] == "github" || envMap["GH_ACCEPTANCE_ORG"] == "cli" {
		return fmt.Errorf("GH_ACCEPTANCE_ORG cannot be 'github' or 'cli'")
	}

	e.host = envMap["GH_ACCEPTANCE_HOST"]
	e.org = envMap["GH_ACCEPTANCE_ORG"]
	e.token = envMap["GH_ACCEPTANCE_TOKEN"]

	e.script = os.Getenv("GH_ACCEPTANCE_SCRIPT")
	e.preserveWorkDir = os.Getenv("GH_ACCEPTANCE_PRESERVE_WORK_DIR") == "true"
	e.skipDefer = os.Getenv("GH_ACCEPTANCE_SKIP_DEFER") == "true"

	return nil
}
