//go:build unix

package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

func outputGridFixture() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if os.Getenv("HOME") != filepath.Join(filepath.Dir(cwd), "home") ||
		os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" ||
		!term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("synthetic Go fixture requires its isolated owned terminal")
	}
	var grid strings.Builder
	for row := range 36 {
		fmt.Fprintf(&grid, "\x1b[%d;1H", row+1)
		for column := range 110 {
			fmt.Fprintf(&grid, "\x1b[38;5;%dmx", 21+column%2*175)
		}
	}
	grid.WriteString("\x1b[0m\x1b[38;1HREADY\r\n")
	if _, err := io.WriteString(os.Stdout, grid.String()); err != nil {
		return err
	}
	time.Sleep(30 * time.Second)
	return nil
}

func nativeOutputBudgetCases(t *testing.T) {
	receipt := os.Getenv("CLI_EXERCISE_TEST_RECEIPT")
	if receipt == "" {
		t.Skip("Select a genuine ready receipt for bounded Node output tests.")
	}
	skillRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, name := range []string{"drained output", "continuously drained output", "stalled output", "permanently stalled output", "closed output"} {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			for _, directory := range []string{"home", "work"} {
				require.NoError(t, os.Mkdir(filepath.Join(root, directory), 0o700))
			}
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			adapter, err := FromReceipt(ctx, skillRoot, receipt)
			require.NoError(t, err)
			command := exec.CommandContext(ctx, adapter.node, adapter.script, adapter.entry)
			command.Dir, command.Env = filepath.Join(root, "work"), []string{"HOME=" + filepath.Join(root, "home")}
			command.Stderr = io.Discard
			cliutil.OwnProcessGroup(command)
			input, err := command.StdinPipe()
			require.NoError(t, err)
			output, err := command.StdoutPipe()
			require.NoError(t, err)
			require.NoError(t, command.Start())
			group, waited := 0, false
			t.Cleanup(func() {
				if group > 1 {
					cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
					require.NoError(t, cliutil.KillProcessGroup(cleanup, group))
					stop()
				}
				if !waited {
					require.NoError(t, cliutil.KillOwned(command))
					_ = input.Close()
					_ = output.Close()
					_ = command.Wait()
				}
			})
			encoder := json.NewEncoder(input)
			decoder := json.NewDecoder(bufio.NewReader(output))
			require.NoError(t, encoder.Encode(map[string]any{
				"id": "open", "op": "open", "launch": LaunchOptions{
					Executable: executable, Args: []string{}, Cwd: command.Dir,
					Env: map[string]string{"HOME": filepath.Join(root, "home"),
						"CLI_EXERCISE_TEST_FIXTURE": "output-grid", "GORACE": "atexit_sleep_ms=0"},
					Columns: 120, Rows: 40,
				},
			}))
			ready, opened := false, false
			for !ready || !opened {
				var message struct {
					Type, ID, Raw string
					OK            bool
					PID           int
				}
				require.NoError(t, decoder.Decode(&message))
				if message.Type == "process_group" {
					group = message.PID
				}
				ready = ready || strings.Contains(message.Raw, "READY")
				opened = opened || message.Type == "response" && message.ID == "open" && message.OK
			}
			require.Greater(t, group, 1)
			resize := func(id string) error {
				return encoder.Encode(map[string]any{"id": id, "op": "input",
					"action": map[string]any{"type": "resize", "columns": 120, "rows": 40}})
			}
			require.NoError(t, resize("measure"))
			snapshotBytes := 0
			for {
				var raw json.RawMessage
				require.NoError(t, decoder.Decode(&raw))
				var message struct{ Type, ID string }
				require.NoError(t, json.Unmarshal(raw, &message))
				if message.Type == "data" {
					snapshotBytes = len(raw) + 1
				}
				if message.Type == "response" && message.ID == "measure" {
					break
				}
			}
			require.Positive(t, snapshotBytes)
			count := 3
			stalled := name == "stalled output" || name == "permanently stalled output"
			if stalled || name == "continuously drained output" {
				count = (68<<20)/snapshotBytes + 2
				require.Less(t, count, 10000, "fixture must reach the byte bound without an unbounded request count")
			}
			if name == "closed output" {
				require.NoError(t, output.Close())
			}
			writes := make(chan error, 1)
			go func() {
				var err error
				for index := range count {
					if err = resize(strconv.Itoa(index)); err != nil {
						break
					}
				}
				writes <- errors.Join(err, input.Close())
			}()
			if stalled || name == "closed output" {
				require.Eventually(t, func() bool {
					return errors.Is(syscall.Kill(-group, 0), syscall.ESRCH)
				}, 15*time.Second, 10*time.Millisecond, "the owned target must stop without waiting for stdout to drain")
				group = 0
			}
			totalBytes, failures := 0, 0
			if name != "closed output" && name != "permanently stalled output" {
				spool, err := os.CreateTemp(root, "protocol-")
				require.NoError(t, err)
				defer spool.Close()
				_, err = io.Copy(spool, decoder.Buffered())
				require.NoError(t, err)
				_, err = io.Copy(spool, output)
				require.NoError(t, err)
				_, err = spool.Seek(0, io.SeekStart)
				require.NoError(t, err)
				decoder = json.NewDecoder(spool)
				for {
					var raw json.RawMessage
					err := decoder.Decode(&raw)
					if errors.Is(err, io.EOF) {
						break
					}
					require.NoError(t, err)
					totalBytes += len(raw) + 1
					var message struct{ Type, Code string }
					require.NoError(t, json.Unmarshal(raw, &message))
					if message.Type == "error" {
						require.Equal(t, "adapter_output_limit", message.Code)
						failures++
					}
				}
			}
			waitErr := command.Wait()
			waited = true
			require.NoError(t, ctx.Err(), "the adapter must finish without the test's emergency timeout")
			t.Logf("delivered %d protocol bytes with %d capture-failure notices; adapter exit %d",
				totalBytes, failures, command.ProcessState.ExitCode())
			if name == "drained output" || name == "continuously drained output" {
				require.NoError(t, waitErr)
				require.NoError(t, <-writes)
				require.Zero(t, failures)
				if name == "continuously drained output" {
					require.Greater(t, totalBytes, 64<<20, "the budget limits pending bytes, not total lifetime output")
				}
				require.ErrorIs(t, syscall.Kill(-group, 0), syscall.ESRCH)
				group = 0
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, waitErr, &exit)
				require.NotZero(t, exit.ExitCode())
				select {
				case <-writes:
				case <-ctx.Done():
					t.Fatal("the input writer did not stop after capture failed")
				}
				if name == "stalled output" {
					require.Equal(t, 1, failures, "overflow must produce one explicit capture failure")
					require.LessOrEqual(t, totalBytes, (64<<20)+snapshotBytes+65536,
						"pending protocol bytes must stay bounded apart from the OS pipe")
				}
			}
		})
	}
}
