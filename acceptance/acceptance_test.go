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
	if repositoryCreationIsManaged(os.Args[1:], os.Getenv("GH_ACCEPTANCE_FIXTURE_MODE")) {
		fmt.Fprintln(os.Stderr, "gh repo create requires 'fixture-repo none'")
		return 1
	}
	return int(ghcmd.Main())
}

func repositoryCreationIsManaged(args []string, fixtureMode string) bool {
	return len(args) > 1 && args[0] == "repo" && args[1] == "create" && fixtureMode != "none"
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

func TestRepositoryCreationIsManaged(t *testing.T) {
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "shared"))
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "isolated"))
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "undeclared"))
	assert.False(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "none"))
	assert.False(t, repositoryCreationIsManaged([]string{"repo", "view", "example"}, "shared"))
}

func TestAcceptance(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	fixtureRepositories, err := newFixtureRepositoryManager(tsEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixtureRepositories.cleanup(); err != nil {
			t.Errorf("cleaning up fixture repositories: %v", err)
		}
	})

	testGroups, err := selectAcceptanceTestGroups(
		acceptanceTestGroups(t),
		os.Getenv("GH_ACCEPTANCE_GROUP"),
	)
	if err != nil {
		t.Fatal(err)
	}

	validateAcceptanceScripts(t, tsEnv, testGroups)

	for _, group := range testGroups {
		t.Run(group, func(t *testing.T) {
			testscript.Run(t, testScriptParamsFor(t, tsEnv, fixtureRepositories, group))
		})
	}
}

func validateAcceptanceScripts(t *testing.T, tsEnv testScriptEnv, groups []string) {
	t.Helper()

	for _, group := range groups {
		candidates, _, err := acceptanceScriptCandidates(tsEnv, group)
		require.NoError(t, err)
		for _, file := range candidates {
			require.NoError(t, validateFixtureRepositoryDeclaration(file))
			_, err := requiresUserCapabilityForScript(file)
			require.NoError(t, err)
		}
	}
}

func acceptanceTestGroups(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	var groups []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		files, err := filepath.Glob(filepath.Join("testdata", entry.Name(), "*.txtar"))
		require.NoError(t, err)
		if len(files) > 0 {
			groups = append(groups, entry.Name())
		}
	}
	require.NotEmpty(t, groups)
	return groups
}

func selectAcceptanceTestGroups(available []string, requested string) ([]string, error) {
	if requested == "" || requested == "all" {
		return available, nil
	}

	for _, group := range available {
		if group == requested {
			return []string{requested}, nil
		}
	}

	return nil, fmt.Errorf("unknown acceptance test group %q; available groups: %s", requested, strings.Join(available, ", "))
}

func TestSelectAcceptanceTestGroups(t *testing.T) {
	available := []string{"api", "pr", "repo"}

	tests := []struct {
		name      string
		requested string
		want      []string
		wantErr   string
	}{
		{name: "empty selects all", want: available},
		{name: "all selects all", requested: "all", want: available},
		{name: "group selects one", requested: "pr", want: []string{"pr"}},
		{name: "unknown group errors", requested: "pull-request", wantErr: `unknown acceptance test group "pull-request"; available groups: api, pr, repo`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectAcceptanceTestGroups(available, tt.requested)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func testScriptParamsFor(t *testing.T, tsEnv testScriptEnv, fixtureRepositories *fixtureRepositoryManager, command string) testscript.Params {
	t.Helper()

	candidates, filtered, err := acceptanceScriptCandidates(tsEnv, command)
	if err != nil {
		t.Fatal(err)
	}
	if filtered && len(candidates) == 0 {
		t.Skipf("testdata/%s: no selected script belongs to this command directory", command)
	}

	files := make([]string, 0, len(candidates))
	for _, file := range candidates {
		requiresUserCapability, err := requiresUserCapabilityForScript(file)
		if err != nil {
			t.Fatal(err)
		}
		if requiresUserCapability && !tsEnv.hasUserCapability {
			if filtered {
				t.Fatalf("%s requires a token that authenticates a user", file)
			}
			continue
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Skip("all scripts require a token that authenticates a user")
	}

	return testscript.Params{
		Files:               files,
		Setup:               sharedSetup(tsEnv),
		Cmds:                sharedCmds(tsEnv, fixtureRepositories),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		TestWork:            tsEnv.preserveWorkDir,
	}
}

func acceptanceScriptCandidates(tsEnv testScriptEnv, command string) ([]string, bool, error) {
	files, filtered := selectScripts(command, tsEnv.scripts)
	if filtered {
		return files, true, nil
	}
	files, err := filepath.Glob(filepath.Join("testdata", command, "*.txtar"))
	return files, false, err
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

		if tsEnv.apiHost == "" {
			ts.Setenv("GH_TOKEN", tsEnv.token)
		} else {
			// api_host is only readable from hosts.yml, and a GH_TOKEN in the
			// environment resolves auth without ever consulting that file, so
			// the token has to move into the same place as the override.
			hostsFile := filepath.Join(ts.Cd, "hosts.yml")
			hostsContent := fmt.Sprintf(""+
				"%[1]s:\n"+
				"    user: %[2]s\n"+
				"    oauth_token: %[3]s\n"+
				"    git_protocol: https\n"+
				"    api_host: %[4]s\n"+
				"    users:\n"+
				"        %[2]s:\n"+
				"            oauth_token: %[3]s\n",
				tsEnv.host, tsEnv.user, tsEnv.token, tsEnv.apiHost)
			if err := os.WriteFile(hostsFile, []byte(hostsContent), 0o600); err != nil {
				return fmt.Errorf("writing sandbox hosts.yml: %w", err)
			}
		}
		ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", "undeclared")

		ts.Setenv("RANDOM_STRING", randomString(10))

		ts.Setenv("GH_TELEMETRY", "false")

		// testscript constructs a fresh environment from a fixed allowlist and
		// does not propagate SSL_CERT_FILE. When the operator has set it - for
		// instance because all API traffic routes through a gateway whose CA is
		// not in the system bundle - honour that intent explicitly, or every
		// request inside the sandbox will fail certificate verification.
		if certFile := os.Getenv("SSL_CERT_FILE"); certFile != "" {
			ts.Setenv("SSL_CERT_FILE", certFile)
		}

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
func sharedCmds(tsEnv testScriptEnv, fixtureRepositories *fixtureRepositoryManager) map[string]func(ts *testscript.TestScript, neg bool, args []string) {
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
				if args[0] == "cleanup-repo" {
					if len(args) != 2 {
						tt.Fatal("usage: defer cleanup-repo REPO")
						return
					}
					if err := fixtureRepositories.delete(args[1]); err != nil {
						tt.Fatal(err)
					}
					return
				}

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
		"fixture-repo": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! fixture-repo")
			}
			if len(args) == 1 && args[0] == "none" {
				ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", "none")
				return
			}
			if len(args) != 2 || (args[0] != "shared" && args[0] != "isolated") {
				ts.Fatalf("usage: fixture-repo (shared|isolated) ENV_VAR, or fixture-repo none")
			}

			repository, err := fixtureRepositories.repository(args[0])
			ts.Check(err)
			ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", args[0])
			ts.Setenv(args[1], repository)
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
	host              string
	org               string
	token             string
	user              string
	hasUserCapability bool

	// scripts optionally narrows a run to named scripts within the command
	// directory being run. Empty means run every script in the directory.
	scripts []string

	// apiHost, when set, routes API traffic through that hostname by writing a
	// hosts.yml instead of authenticating from GH_TOKEN. Used by the gateway
	// harness in script/api-host-gateway.
	apiHost string

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
	var err error
	e.hasUserCapability, err = tokenHasUserCapability(e.token)
	if err != nil {
		return err
	}

	e.scripts = parseScriptFilter(os.Getenv("GH_ACCEPTANCE_SCRIPT"))
	e.preserveWorkDir = os.Getenv("GH_ACCEPTANCE_PRESERVE_WORK_DIR") == "true"
	e.skipDefer = os.Getenv("GH_ACCEPTANCE_SKIP_DEFER") == "true"
	e.apiHost = os.Getenv("GH_ACCEPTANCE_API_HOST")
	e.user = os.Getenv("GH_ACCEPTANCE_USER")
	if e.apiHost != "" && e.user == "" {
		return fmt.Errorf("GH_ACCEPTANCE_USER is required when GH_ACCEPTANCE_API_HOST is set")
	}

	return nil
}
