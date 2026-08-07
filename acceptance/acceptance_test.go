//go:build acceptance

package acceptance_test

import (
	"bytes"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"math/rand"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghcmd"
	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-internal/testscript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func ghMain() int {
	return int(ghcmd.Main())
}

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"gh": ghMain,
	}))
}

func TestGenerateSSHPublicKey(t *testing.T) {
	first, err := generateSSHPublicKey("myTitle")
	require.NoError(t, err)
	second, err := generateSSHPublicKey("myTitle")
	require.NoError(t, err)

	publicKey, comment, options, rest, err := ssh.ParseAuthorizedKey(first)
	require.NoError(t, err)
	assert.Equal(t, ssh.KeyAlgoED25519, publicKey.Type())
	assert.Equal(t, "myTitle", comment)
	assert.Empty(t, options)
	assert.Empty(t, rest)
	assert.NotEqual(t, first, second)
}

func TestSandboxFilePath(t *testing.T) {
	root := t.TempDir()

	path, err := sandboxFilePath(root, root, "keys/deploy.pub")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "keys/deploy.pub"), path)

	_, err = sandboxFilePath(root, root, filepath.Join(root, "deploy.pub"))
	assert.EqualError(t, err, "path must be relative to the testscript sandbox")

	_, err = sandboxFilePath(root, root, "../deploy.pub")
	assert.EqualError(t, err, "path must stay within the testscript sandbox")
}

func TestAPI(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "api"))
}

func TestAuth(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "auth"))
}

func TestGists(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "gist"))
}

func TestGPGKeys(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "gpg-key"))
}

func TestExtensions(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "extension"))
}

func TestIssues(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "issue"))
}

func TestDiscussions(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "discussion"))
}

func TestIssues2_0(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "issues-2.0"))
}

func TestLabels(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "label"))
}

func TestOrg(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "org"))
}

func TestProject(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "project"))
}

func TestPullRequests(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "pr"))
}

func TestReleases(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "release"))
}

func TestRepo(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "repo"))
}

func TestRulesets(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "ruleset"))
}

func TestSearches(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "search"))
}

func TestSecrets(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "secret"))
}

func TestSSHKeys(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "ssh-key"))
}

func TestVariables(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "variable"))
}

func TestWorkflows(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "workflow"))
}

func TestTelemetry(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	testscript.Run(t, testScriptParamsFor(t, tsEnv, "telemetry"))
}

func testScriptParamsFor(t *testing.T, tsEnv testScriptEnv, command string) testscript.Params {
	t.Helper()
	files, filtered := selectScripts(command, tsEnv.scripts)

	var dir string
	if !filtered {
		// No filter was set - run everything in the directory.
		dir = path.Join("testdata", command)
	} else if len(files) == 0 {
		// A filter was set but none of the selected scripts belong to this
		// command directory, so skip rather than running the whole directory.
		t.Skipf("testdata/%s: no selected script belongs to this command directory", command)
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

		// When using script name to uniquely identify where test data comes from,
		// some places like GitHub Actions secret names don't accept hyphens.
		// Replace them with underscores until such a time this becomes a problem.
		ts.Setenv("SCRIPT_NAME", strings.ReplaceAll(scriptName, "-", "_"))
		ts.Setenv("HOME", ts.Cd)
		ts.Setenv("GH_CONFIG_DIR", ts.Cd)

		ts.Setenv("GH_HOST", tsEnv.host)
		ts.Setenv("ORG", tsEnv.org)
		ts.Setenv("GH_TOKEN", tsEnv.token)

		ts.Setenv("RANDOM_STRING", randomString(10))

		ts.Setenv("GH_TELEMETRY", "false")

		// The sandbox overrides HOME, so git cannot find the user's global
		// config. Write a minimal identity so commits inside the sandbox
		// don't fail with "Author identity unknown".
		gitCfg := filepath.Join(ts.Cd, ".gitconfig")
		gitCfgContent := heredoc.Doc(`
			[user]
				name = GitHub CLI Acceptance Test Runner
				email = cli-acceptance-test-runner@github.com
		`)
		if err := os.WriteFile(gitCfg, []byte(gitCfgContent), 0o644); err != nil {
			return fmt.Errorf("writing sandbox .gitconfig: %w", err)
		}

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
		"env2upper": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! env2upper")
			}
			if len(args) == 0 {
				ts.Fatalf("usage: env2upper name=value ...")
			}
			for _, env := range args {
				i := strings.Index(env, "=")

				if i < 0 {
					ts.Fatalf("env2upper: argument does not match name=value")
				}

				ts.Setenv(env[:i], strings.ToUpper(env[i+1:]))
			}
		},
		"generate-ssh-key": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! generate-ssh-key")
			}
			if len(args) < 1 || len(args) > 2 {
				ts.Fatalf("usage: generate-ssh-key file [comment]")
			}

			comment := ""
			if len(args) == 2 {
				comment = args[1]
			}
			publicKey, err := generateSSHPublicKey(comment)
			ts.Check(err)
			outputPath, err := sandboxFilePath(ts.Getenv("WORK"), ts.MkAbs("."), args[0])
			ts.Check(err)
			ts.Check(os.WriteFile(outputPath, publicKey, 0o644))
		},
		"replace": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! replace")
			}
			if len(args) < 2 {
				ts.Fatalf("usage: replace file env...")
			}

			src := ts.MkAbs(args[0])
			ts.Logf("replace src: %s", src)

			// Preserve the existing file mode while replacing the contents similar to native cp behavior
			info, err := os.Stat(src)
			ts.Check(err)
			mode := info.Mode() & 0o777
			data, err := os.ReadFile(src)
			ts.Check(err)

			for _, arg := range args[1:] {
				i := strings.Index(arg, "=")
				if i < 0 {
					ts.Fatalf("replace: %s argument does not match name=value", arg)
				}

				name := fmt.Sprintf("$%s", arg[:i])
				value := arg[i+1:]
				ts.Logf("replace %s: %s", name, value)

				// `replace` was originally built similar to `cp` and `cmpenv`, expanding environment variables within a file.
				// However files with content that looks like environments variable such as GitHub Actions workflows
				// were being modified unexpectedly. Thus `replace` has been designed to using string replacement
				// looking for `$KEY` specifically.
				data = []byte(strings.ReplaceAll(string(data), name, value))
			}

			ts.Check(os.WriteFile(src, data, mode))
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
		"jq-assert": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! jq-assert")
			}
			if len(args) != 3 {
				ts.Fatalf("usage: jq-assert ENV_VAR expression regexp")
			}

			input := ts.Getenv(args[0])
			if input == "" {
				ts.Fatalf("jq-assert: environment variable %s is empty or unset", args[0])
			}

			var buf bytes.Buffer
			if err := jq.Evaluate(strings.NewReader(input), &buf, args[1]); err != nil {
				ts.Fatalf("jq-assert: %v", err)
			}

			result := strings.TrimRight(buf.String(), "\n") // jq.Evaluate adds a newline at the end
			ts.Logf("jq-assert %s %q => %s", args[0], args[1], result)

			re, err := regexp.Compile(args[2])
			if err != nil {
				ts.Fatalf("jq-assert: invalid regexp %q: %v", args[2], err)
			}
			if !re.MatchString(result) {
				ts.Fatalf("jq-assert: result %q does not match %q", result, args[2])
			}
		},
		"jq2env": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! jq2env")
			}
			if len(args) != 3 {
				ts.Fatalf("usage: jq2env SRC_ENV expression DST_ENV")
			}

			input := ts.Getenv(args[0])
			if input == "" {
				ts.Fatalf("jq2env: environment variable %s is empty or unset", args[0])
			}

			var buf bytes.Buffer
			if err := jq.Evaluate(strings.NewReader(input), &buf, args[1]); err != nil {
				ts.Fatalf("jq2env: %v", err)
			}

			result := strings.TrimRight(buf.String(), "\n") // jq.Evaluate adds a newline at the end
			ts.Logf("jq2env %s %q => %s => %s", args[0], args[1], result, args[2])
			ts.Setenv(args[2], result)
		},
	}
}

func generateSSHPublicKey(comment string) ([]byte, error) {
	publicKey, _, err := ed25519.GenerateKey(cryptorand.Reader)
	if err != nil {
		return nil, err
	}

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	authorizedKey := bytes.TrimSpace(ssh.MarshalAuthorizedKey(sshPublicKey))
	if comment != "" {
		authorizedKey = append(authorizedKey, ' ')
		authorizedKey = append(authorizedKey, comment...)
	}
	return append(authorizedKey, '\n'), nil
}

func sandboxFilePath(root, currentDir, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", errors.New("path must be relative to the testscript sandbox")
	}

	outputPath := filepath.Clean(filepath.Join(currentDir, name))
	relativePath, err := filepath.Rel(root, outputPath)
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay within the testscript sandbox")
	}
	return outputPath, nil
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

	// scripts optionally narrows a run to named scripts within the command
	// directory being run. Empty means run every script in the directory.
	scripts []string

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

	e.scripts = parseScriptFilter(os.Getenv("GH_ACCEPTANCE_SCRIPT"))
	e.preserveWorkDir = os.Getenv("GH_ACCEPTANCE_PRESERVE_WORK_DIR") == "true"
	e.skipDefer = os.Getenv("GH_ACCEPTANCE_SKIP_DEFER") == "true"

	return nil
}

func TestSkills(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testScriptParamsFor(t, tsEnv, "skills"))
}
