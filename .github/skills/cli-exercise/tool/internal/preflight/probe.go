package preflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/cli/cli/v2/cli-exercise/internal/terminal"
	"golang.org/x/term"
)

func checkTerminal(ctx context.Context, session terminal.Session, launch terminal.LaunchOptions) (err error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, session.Close(cleanup))
	}()
	if err := session.Start(ctx, launch); err != nil {
		return err
	}
	var latest recording.TerminalData
	var raw strings.Builder
	var exitCode *int
	exited := false
	observe := func(ready func() bool) error {
		for !ready() {
			select {
			case <-ctx.Done():
				return fmt.Errorf("terminal capability probe timed out: %w", ctx.Err())
			case event, open := <-session.Events():
				if !open {
					return fmt.Errorf("terminal adapter closed before the probe completed")
				}
				switch event.Kind {
				case "data":
					if err := json.Unmarshal(event.Data, &latest); err != nil {
						return err
					}
					if _, err := latest.VisibleText(); err != nil {
						return err
					}
					if len(latest.Cursor) != 2 || latest.CursorVisible == nil || latest.CursorStyle == "" ||
						latest.Offset == nil || latest.TotalLines == nil {
						return fmt.Errorf("terminal capability metadata is incomplete")
					}
					for _, line := range latest.Lines {
						for _, span := range line.Spans {
							if span.Width == nil {
								return fmt.Errorf("terminal span width is missing")
							}
						}
					}
					if raw.Len()+len(event.Raw) > 1<<20 {
						return fmt.Errorf("terminal probe exceeded its output bound")
					}
					raw.WriteString(event.Raw)
				case "exit":
					exited, exitCode = true, event.ExitCode
					if exitCode == nil || *exitCode != 0 || event.Signal != nil && *event.Signal != 0 {
						return fmt.Errorf("the owned probe fixture did not exit successfully")
					}
				default:
					return errors.Join(fmt.Errorf("terminal capability probe failed"), event.Err)
				}
			}
		}
		return nil
	}
	if err := observe(func() bool { return strings.Contains(raw.String(), "CLI_EXERCISE_READY") }); err != nil {
		return err
	}
	if err := session.Input(ctx, json.RawMessage(`{"type":"resize","columns":44,"rows":12}`)); err != nil {
		return err
	}
	if err := observe(func() bool { return latest.Columns == 44 && latest.Rows == 12 }); err != nil {
		return err
	}
	for _, input := range []string{
		`{"type":"text","text":"p"}`, `{"type":"key","key":"r"}`, `{"type":"text","text":"ob"}`,
		`{"type":"key","key":"e"}`, `{"type":"key","key":"enter"}`,
	} {
		if err := session.Input(ctx, json.RawMessage(input)); err != nil {
			return err
		}
	}
	if err := observe(func() bool { return exited }); err != nil {
		return err
	}
	text, err := latest.VisibleText()
	if err != nil {
		return err
	}
	if !strings.Contains(raw.String(), "CLI_EXERCISE_ACK") || !strings.Contains(text, "CLI_EXERCISE_ACK") {
		return fmt.Errorf("the adapter did not retain both raw and visible fixture output")
	}
	return nil
}

func probeFixture(ctx context.Context, input *os.File, output io.Writer) (err error) {
	if os.Getenv("CLI_EXERCISE_PROBE") != "approved" || !term.IsTerminal(int(input.Fd())) {
		return fmt.Errorf("probe fixture requires its approved, owned terminal")
	}
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, term.Restore(int(input.Fd()), state)) }()
	if _, err := io.WriteString(output, "CLI_EXERCISE_READY\r\n"); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	type readResult struct {
		data byte
		err  error
	}
	inputs := make(chan readResult)
	go func() {
		for {
			var data [1]byte
			_, err := io.ReadFull(input, data[:])
			select {
			case inputs <- readResult{data: data[0], err: err}:
			case <-bounded.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	var received []byte
	for {
		select {
		case <-bounded.Done():
			return fmt.Errorf("owned probe fixture exceeded its deadline")
		case value := <-inputs:
			if value.err != nil {
				return value.err
			}
			received = append(received, value.data)
			if value.data == '\r' {
				if string(received) != "probe\r" {
					return fmt.Errorf("owned probe fixture received unexpected input")
				}
				_, err := io.WriteString(output, "CLI_EXERCISE_ACK\r\n")
				return err
			}
			if len(received) > len("probe") {
				return fmt.Errorf("owned probe fixture received excessive input")
			}
			if _, err := io.WriteString(output, "CLI_EXERCISE_PROGRESS\r\n"); err != nil {
				return err
			}
		}
	}
}

// ProbeMain is the private Go subject used by the Tuistory capability check.
func ProbeMain(ctx context.Context, args []string, streams cliutil.Streams) int {
	input, ok := streams.In.(*os.File)
	if !ok || len(args) != 0 {
		if err := cliutil.Error(streams.ErrOut, "probe_fixture_failed", "The owned probe requires terminal stdin and no arguments."); err != nil {
			return 1
		}
		return 1
	}
	if err := probeFixture(ctx, input, streams.Out); err != nil {
		if writeErr := cliutil.Error(streams.ErrOut, "probe_fixture_failed", err.Error()); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
}
