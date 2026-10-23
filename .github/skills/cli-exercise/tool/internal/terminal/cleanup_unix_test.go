//go:build unix

package terminal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

type fixtureProcesses struct {
	Leader int `json:"leader"`
	Child  int `json:"child"`
}

func descendantFixture(mode string) error {
	if mode == "descendant-child" {
		signal.Ignore(syscall.SIGHUP)
		if os.Getenv("CLI_EXERCISE_IGNORE_TERM") == "true" {
			signal.Ignore(syscall.SIGTERM)
		}
		if _, err := io.WriteString(os.Stdout, "READY\n"); err != nil {
			return err
		}
		time.Sleep(30 * time.Second)
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if os.Getenv("HOME") != filepath.Join(filepath.Dir(cwd), "home") ||
		os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" ||
		!term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("synthetic Go fixture requires its isolated owned terminal")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	child := exec.Command(executable)
	child.Env = []string{"CLI_EXERCISE_TEST_FIXTURE=descendant-child", "GORACE=atexit_sleep_ms=0",
		"CLI_EXERCISE_IGNORE_TERM=" + os.Getenv("CLI_EXERCISE_IGNORE_TERM")}
	output, err := child.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	if err := child.Start(); err != nil {
		return err
	}
	ready, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || ready != "READY\n" {
		return errors.Join(fmt.Errorf("descendant readiness failed"), err, child.Process.Kill(), child.Wait())
	}
	err = cliutil.WriteJSON(filepath.Join(cwd, "processes.json"), fixtureProcesses{Leader: os.Getpid(), Child: child.Process.Pid})
	if err == nil {
		_, err = fmt.Fprintf(os.Stdout, "CHILD:%d\r\n", child.Process.Pid)
	}
	if err != nil {
		return errors.Join(err, child.Process.Kill(), child.Wait())
	}
	if os.Getenv("CLI_EXERCISE_LEADER") == "wait" {
		return child.Wait()
	}
	return child.Process.Release()
}

func nativeCleanupCases(t *testing.T) {
	receipt := os.Getenv("CLI_EXERCISE_TEST_RECEIPT")
	if receipt == "" {
		t.Skip("Select a genuine ready receipt for owned PTY cleanup tests.")
	}
	skillRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name       string
		leader     string
		ignoreTerm bool
		cancel     bool
		crash      bool
		eof        bool
	}{
		{name: "exited leader with HUP-resistant child", leader: "exit"},
		{name: "exited leader with TERM-resistant child", leader: "exit", ignoreTerm: true},
		{name: "live leader with TERM-resistant child", leader: "wait", ignoreTerm: true},
		{name: "cancelled cleanup with TERM-resistant child", leader: "wait", ignoreTerm: true, cancel: true},
		{name: "adapter crash with TERM-resistant child", leader: "wait", ignoreTerm: true, crash: true},
		{name: "owner pipe closed with TERM-resistant child", leader: "wait", ignoreTerm: true, eof: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			for _, directory := range []string{"home", "work"} {
				require.NoError(t, os.Mkdir(filepath.Join(root, directory), 0o700))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			adapter, err := FromReceipt(ctx, skillRoot, receipt)
			require.NoError(t, err)
			capture := &adapterCapture{done: make(chan struct{})}
			go func() {
				defer close(capture.done)
				for event := range adapter.Events() {
					capture.mu.Lock()
					capture.raw += event.Raw
					capture.events = append(capture.events, event)
					capture.mu.Unlock()
				}
			}()
			closed, childStopped := false, false
			t.Cleanup(func() {
				var processes fixtureProcesses
				_, readErr := cliutil.ReadJSON(filepath.Join(root, "work", "processes.json"), 4096, &processes)
				if !childStopped && readErr == nil && processes.Child > 1 {
					err := syscall.Kill(processes.Child, syscall.SIGKILL)
					if !errors.Is(err, syscall.ESRCH) {
						require.NoError(t, err)
					}
					require.Eventually(t, func() bool {
						return errors.Is(syscall.Kill(processes.Child, 0), syscall.ESRCH)
					}, 3*time.Second, 10*time.Millisecond, "explicit fixture child cleanup failed")
				}
				if !closed {
					cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
					defer stop()
					_ = adapter.Close(cleanup)
				}
			})
			require.NoError(t, adapter.Start(ctx, LaunchOptions{
				Executable: executable, Args: []string{}, Cwd: filepath.Join(root, "work"),
				Env: map[string]string{"HOME": filepath.Join(root, "home"),
					"CLI_EXERCISE_TEST_FIXTURE": "descendants", "CLI_EXERCISE_LEADER": tc.leader,
					"CLI_EXERCISE_IGNORE_TERM": fmt.Sprint(tc.ignoreTerm), "GORACE": "atexit_sleep_ms=0"},
				Columns: 80, Rows: 24,
			}))
			capture.wait(t, "CHILD:")
			var processes fixtureProcesses
			_, err = cliutil.ReadJSON(filepath.Join(root, "work", "processes.json"), 4096, &processes)
			require.NoError(t, err)
			require.Greater(t, processes.Child, 1)
			group, err := syscall.Getpgid(processes.Child)
			require.NoError(t, err)
			require.Equal(t, processes.Leader, group, "the child must belong to the owned PTY group")
			if tc.leader == "exit" {
				require.Eventually(t, func() bool {
					capture.mu.Lock()
					defer capture.mu.Unlock()
					for _, event := range capture.events {
						if event.Kind == "exit" && event.ExitCode != nil && *event.ExitCode == 0 {
							return true
						}
					}
					return false
				}, 3*time.Second, 10*time.Millisecond, "the PTY leader must already have exited")
			}
			require.NoError(t, syscall.Kill(processes.Child, 0), "the descendant must still be alive before cleanup")
			if tc.crash {
				require.NoError(t, adapter.command.Process.Kill())
			}
			if tc.eof {
				require.NoError(t, adapter.input.Close())
				select {
				case <-adapter.done:
				case <-ctx.Done():
					t.Fatal("adapter did not clean up after its owner closed the pipe")
				}
			}
			cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if tc.cancel {
				stop()
			}
			err = adapter.Close(cleanup)
			closed = true
			if tc.cancel || tc.crash {
				require.Error(t, err, "interrupted cleanup must not report success")
			} else {
				require.NoError(t, err)
			}
			require.ErrorIs(t, syscall.Kill(-processes.Leader, 0), syscall.ESRCH, "cleanup returned before its process group was gone")
			require.ErrorIs(t, syscall.Kill(processes.Child, 0), syscall.ESRCH, "cleanup returned with a descendant still running")
			childStopped = true
			select {
			case <-capture.done:
			case <-time.After(time.Second):
				t.Fatal("owned adapter event reader did not close")
			}
			require.NotNil(t, adapter.command.ProcessState)
		})
	}
}
