package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	expect "github.com/Netflix/go-expect"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/safeexec"
	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/assert"
)

func TestRepoFork(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("skipping test in windows")
	}
	setupEnvVars(t)
	setupTempDir(t)
	reg := httpmock.Registry{}
	// All requests to .git paths return blank success responses.
	reg.Register(httpmock.Git(), httpmock.StringResponse(""))
	forkPath := "repos/owner/repo/forks"
	forkResult := fmt.Sprintf(`{
	    "node_id": "123",
	    "name": "repo",
	    "clone_url": "https://github.com/someone/repo.git",
	    "created_at": "%s",
	    "owner": {
	      "login": "someone"
	    }
	  }`, time.Now().Format(time.RFC3339))
	reg.Register(httpmock.REST("POST", forkPath), httpmock.StringResponse(forkResult))
	setupServer(t, &reg)
	config := ""
	hosts := heredoc.Doc(`
    github.localhost:
      user: monalisa
      oauth_token: TOKEN
      git_protocol: https
	`)
	setupConfigFiles(t, config, hosts)
	repoDir := setupLocalRepo(t, "owner", "repo")
	assert.NoError(t, os.Chdir(repoDir))
	consoleOutput := &bytes.Buffer{}
	c := setupVirtualConsole(t, consoleOutput)

	donec := make(chan struct{})
	go func() {
		defer close(donec)
		// Define virtual console interactivity.
		_, _ = c.ExpectString("Would you like to add a remote for the fork?")
		_, _ = c.SendLine("Y")
		// Expect string that never gets output by command, this forces go-expect
		// to keep reading the command output to the buffer. Set the read timeout
		// to assume command has finished when there is no output for 0.5 seconds.
		_, _ = c.Expect(expect.String("FINISHED"), expect.WithTimeout(500*time.Millisecond))
	}()
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"gh", "repo", "fork"}
	// Execute command specified by os.Args.
	code := mainRun()
	// Wait for virtual console to finish.
	<-donec

	assert.Equal(t, exitOK, code)
	out := stripANSI(t, consoleOutput.String())
	assert.Regexp(t, "✓ Created fork someone/repo", out)
	assert.Regexp(t, "✓ Added remote origin", out)
}

// setupTempDir creates a temp directory and changes the working
// directory to it. The temp directory gets automatically cleaned
// up at the end of the test, and the working directory gets set back
// to its original value.
func setupTempDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("unable to get working directory: %v", err)
	}
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("unable to change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	return tempDir
}

// setupServer creates a test http server that is backed by httpmock for
// matching requests to responses. Any request that does not match will
// return a 500 response. Both http_proxy and HTTP_PROXY environment
// variables will be set to the new server URL so all network requets will
// be directed to the new server. Note that git only responds to lowercase
// http_proxy variable which is why both are necessary to be set. The
// variables will be automatically reset upon test completion. Lastly,
// by default, the registry will be verified against requests at the end
// of the test, we may want to make this configurable in the future.
func setupServer(t *testing.T, reg *httpmock.Registry) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := reg.RoundTrip(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Copy headers from response to ResponseWriter.
		for k, v := range res.Header {
			for i, p := range v {
				if i == 0 {
					w.Header().Set(k, p)
				} else {
					w.Header().Add(k, p)
				}
			}
		}
		w.WriteHeader(res.StatusCode)
		body, _ := io.ReadAll(res.Body)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	setEnv(t, "HTTP_PROXY", s.URL)
	setEnv(t, "http_proxy", s.URL)
	t.Cleanup(func() { reg.Verify(t) })
	return s
}

// setupLocalRepo initializes a repository in the current directory
// with the given owner and name. It sets up the origin remote to point
// towards http://github.localhost. Additionally, it sets the remote as
// resolved as not to make extra graphql request to resolve it. The
// remotes are not configurable right now but in the future we can
// change that if needed.
func setupLocalRepo(t *testing.T, owner, name string) string {
	t.Helper()
	currentWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("unable to get working directory: %v", err)
	}
	gitExe, err := safeexec.LookPath("git")
	if err != nil {
		t.Fatalf("unable to find git executable: %v", err)
	}
	initCmd := exec.Command(gitExe, "init", "--quiet", name)
	err = initCmd.Run()
	if err != nil {
		t.Fatalf("unable to initialize git repo: %v", err)
	}
	err = os.Chdir(name)
	if err != nil {
		t.Fatalf("unable to change working directory: %v", err)
	}
	url := fmt.Sprintf("%s/%s/%s.git", "http://github.localhost", owner, name)
	remoteAddCmd := exec.Command(gitExe, "remote", "add", "origin", url)
	err = remoteAddCmd.Run()
	if err != nil {
		t.Fatalf("unable to add git remote: %v", err)
	}
	resolveRemoteCmd := exec.Command(gitExe, "config", "--add", "remote.origin.gh-resolved", "base")
	err = resolveRemoteCmd.Run()
	if err != nil {
		t.Fatalf("unable to manually resolve git remote: %v", err)
	}
	err = os.Chdir(currentWd)
	if err != nil {
		t.Fatalf("unable to change working directory: %v", err)
	}
	return filepath.Join(currentWd, name)
}

// setupConfigFile will write config and host strings to config.yml and hosts.yml,
// respectively, in the current directory. It will also set the GH_CONFIG_DIR environment
// variable to the current directory, which will be reset upon test completion.
func setupConfigFiles(t *testing.T, config, hosts string) {
	t.Helper()
	currentWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("unable to get working directory: %v", err)
	}
	setEnv(t, "GH_CONFIG_DIR", currentWd)
	if config != "" {
		err = ioutil.WriteFile("config.yml", []byte(config), 0771)
		if err != nil {
			t.Fatalf("unable to write config.yml: %v", err)
		}
	}
	if hosts != "" {
		err := ioutil.WriteFile("hosts.yml", []byte(hosts), 0771)
		if err != nil {
			t.Fatalf("unable to write hosts.yml: %v", err)
		}
	}
}

// setupVirtualConsole will create a virtual console that for use in tests.
// The virtual console will override the stdout, stderr, and stdin, and reset
// them at the end of the test. As stdout, and stderr are overriden it can be
// difficult to debug a test as standard print statements will not be visible.
// To fix this issue use the verbose option, -v, with go test.
func setupVirtualConsole(t *testing.T, w io.Writer) *expect.Console {
	t.Helper()
	c, _, err := vt10x.NewVT10XConsole(expect.WithStdout(w))
	if err != nil {
		t.Fatalf("unable to create virtual console: %v", err)
	}
	origIn := os.Stdin
	origOut := os.Stdout
	origErr := os.Stderr
	t.Cleanup(func() {
		os.Stdin = origIn
		os.Stdout = origOut
		os.Stderr = origErr
		c.Close()
	})
	os.Stdin = c.Tty()
	os.Stdout = c.Tty()
	os.Stderr = c.Tty()
	return c
}

// setupEnvVars sets environment variables to disable
// functionality we are not testing.
func setupEnvVars(t *testing.T) {
	t.Helper()
	// Skip checking for updates.
	setEnv(t, "GH_NO_UPDATE_NOTIFIER", "1")
	// Disable color output.
	setEnv(t, "NO_COLOR", "1")
	setEnv(t, "CLICOLOR", "0")
}

// setEnv sets an environment variable and will reset
// it at the end of the test.
func setEnv(t *testing.T, key, newValue string) {
	t.Helper()
	oldValue, hasValue := os.LookupEnv(key)
	err := os.Setenv(key, newValue)
	if err != nil {
		t.Fatalf("unable to set environment variable: %v", err)
	}
	t.Cleanup(func() {
		if hasValue {
			os.Setenv(key, oldValue)
		} else {
			os.Unsetenv(key)
		}
	})
}

// stripANSI removes ANSI escape codes from provided string.
func stripANSI(t *testing.T, s string) string {
	t.Helper()
	ansi := "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"
	re := regexp.MustCompile(ansi)
	return re.ReplaceAllString(s, "")
}
