package ghcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_executable(t *testing.T) {
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testExeName := filepath.Base(testExe)

	// Create 3 extra PATH entries that each contain an executable with the same name as the running test
	// process. The first is a symlink, but to an unrelated executable, the second is a symlink to our test
	// process and thus represents the result we want, and the third one is an unrelated executable.
	dir := t.TempDir()
	bin1 := filepath.Join(dir, "bin1")
	bin1Exe := filepath.Join(bin1, testExeName)
	bin2 := filepath.Join(dir, "bin2")
	bin2Exe := filepath.Join(bin2, testExeName)
	bin3 := filepath.Join(dir, "bin3")
	bin3Exe := filepath.Join(bin3, testExeName)

	if err := os.MkdirAll(bin1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin3, 0755); err != nil {
		t.Fatal(err)
	}
	if f, err := os.OpenFile(bin3Exe, os.O_CREATE, 0755); err == nil {
		f.Close()
	} else {
		t.Fatal(err)
	}
	if err := os.Symlink(testExe, bin2Exe); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin3Exe, bin1Exe); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", strings.Join([]string{bin1, bin2, bin3, oldPath}, string(os.PathListSeparator)))

	if got := executable(""); got != bin2Exe {
		t.Errorf("executable() = %q, want %q", got, bin2Exe)
	}
}

func Test_executable_relative(t *testing.T) {
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testExeName := filepath.Base(testExe)

	// Create 3 extra PATH entries that each contain an executable with the same name as the running test
	// process. The first is a relative symlink, but to an unrelated executable, the second is a relative
	// symlink to our test process and thus represents the result we want, and the third one is an unrelated
	// executable.
	dir := t.TempDir()
	bin1 := filepath.Join(dir, "bin1")
	bin1Exe := filepath.Join(bin1, testExeName)
	bin2 := filepath.Join(dir, "bin2")
	bin2Exe := filepath.Join(bin2, testExeName)
	bin3 := filepath.Join(dir, "bin3")
	bin3Exe := filepath.Join(bin3, testExeName)

	if err := os.MkdirAll(bin1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin3, 0755); err != nil {
		t.Fatal(err)
	}

	if f, err := os.OpenFile(bin3Exe, os.O_CREATE, 0755); err == nil {
		f.Close()
	} else {
		t.Fatal(err)
	}
	bin2Rel, err := filepath.Rel(bin2, testExe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin2Rel, bin2Exe); err != nil {
		t.Fatal(err)
	}
	bin1Rel, err := filepath.Rel(bin1, bin3Exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin1Rel, bin1Exe); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", strings.Join([]string{bin1, bin2, bin3, oldPath}, string(os.PathListSeparator)))

	if got := executable(""); got != bin2Exe {
		t.Errorf("executable() = %q, want %q", got, bin2Exe)
	}
}

func TestExecutablePath(t *testing.T) {
	override := strings.Join([]string{"C:", "cygwin64", "home", "gh.exe"}, string(os.PathSeparator))
	t.Setenv("GH_PATH", override)
	require.Equal(t, override, executablePath("gh"))
}

func TestSetExecutablePathForCommand(t *testing.T) {
	tests := []struct {
		name    string
		path    []string
		resolve bool
	}{
		{name: "auth login", path: []string{"auth", "login"}, resolve: true},
		{name: "auth refresh", path: []string{"auth", "refresh"}, resolve: true},
		{name: "auth setup-git", path: []string{"auth", "setup-git"}, resolve: true},
		{name: "gist clone", path: []string{"gist", "clone"}, resolve: true},
		{name: "issue develop", path: []string{"issue", "develop"}, resolve: true},
		{name: "pr create", path: []string{"pr", "create"}, resolve: true},
		{name: "pr checkout", path: []string{"pr", "checkout"}, resolve: true},
		{name: "pr merge", path: []string{"pr", "merge"}, resolve: true},
		{name: "repo create", path: []string{"repo", "create"}, resolve: true},
		{name: "repo clone", path: []string{"repo", "clone"}, resolve: true},
		{name: "repo fork", path: []string{"repo", "fork"}, resolve: true},
		{name: "repo sync", path: []string{"repo", "sync"}, resolve: true},
		{name: "pr list", path: []string{"pr", "list"}},
		{name: "repo view", path: []string{"repo", "view"}},
		{name: "auth status", path: []string{"auth", "status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "gh"}
			cmd := root
			for _, name := range tt.path {
				child := &cobra.Command{Use: name}
				cmd.AddCommand(child)
				cmd = child
			}

			f := &cmdutil.Factory{ExecutablePath: "before", GitClient: &git.Client{GhPath: "before"}}
			resolveCalls := 0
			setExecutablePathForCommand(cmd, f, func() string {
				resolveCalls++
				return "/resolved/gh"
			})

			wantCalls := 0
			wantPath := "before"
			if tt.resolve {
				wantCalls = 1
				wantPath = "/resolved/gh"
			}
			require.Equal(t, wantCalls, resolveCalls)
			require.Equal(t, wantPath, f.ExecutablePath)
			require.Equal(t, wantPath, f.GitClient.GhPath)
		})
	}
}

func TestDeferExecutablePathUntilPreRun(t *testing.T) {
	for _, tt := range []struct {
		name    string
		preErr  error
		wantErr bool
	}{
		{name: "successful pre-run"},
		{name: "failed pre-run", preErr: errors.New("config error"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "gh"}
			auth := &cobra.Command{Use: "auth"}
			login := &cobra.Command{Use: "login"}
			root.AddCommand(auth)
			auth.AddCommand(login)
			f := &cmdutil.Factory{ExecutablePath: "before", GitClient: &git.Client{GhPath: "before"}}
			preRunCalls := 0
			root.PersistentPreRunE = func(*cobra.Command, []string) error {
				preRunCalls++
				require.Equal(t, "before", f.ExecutablePath)
				return tt.preErr
			}
			resolveCalls := 0
			deferExecutablePathUntilPreRun(root, f, func() string {
				resolveCalls++
				return "/resolved/gh"
			})

			err := root.PersistentPreRunE(login, nil)
			if tt.wantErr {
				require.ErrorIs(t, err, tt.preErr)
				require.Equal(t, 0, resolveCalls)
				require.Equal(t, "before", f.ExecutablePath)
				require.Equal(t, "before", f.GitClient.GhPath)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, resolveCalls)
				require.Equal(t, "/resolved/gh", f.ExecutablePath)
				require.Equal(t, "/resolved/gh", f.GitClient.GhPath)
			}
			require.Equal(t, 1, preRunCalls)
		})
	}
}
