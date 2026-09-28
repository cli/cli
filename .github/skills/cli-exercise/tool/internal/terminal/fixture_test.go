package terminal

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/term"
)

func TestMain(tests *testing.M) {
	var err error
	if len(os.Args) == 3 && os.Args[1] == "transport-fixture" {
		var burst int
		burst, err = strconv.Atoi(os.Args[2])
		if err == nil {
			err = transportFixture(burst)
		}
	} else if os.Getenv("CLI_EXERCISE_TEST_FIXTURE") == "bytes" {
		err = rawFixture()
	} else if mode := os.Getenv("CLI_EXERCISE_TEST_FIXTURE"); mode == "descendants" || mode == "descendant-child" {
		err = descendantFixture(mode)
	} else if os.Getenv("CLI_EXERCISE_TEST_FIXTURE") == "output-grid" {
		err = outputGridFixture()
	} else {
		os.Exit(tests.Run())
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func transportFixture(burst int) error {
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	data := func(raw string) error {
		return encoder.Encode(map[string]any{"type": "data", "raw": raw, "data": map[string]any{
			"cols": 80, "rows": 24, "cursor": []int{0, 0}, "cursorVisible": true, "cursorStyle": "block",
			"lines": []any{map[string]any{"spans": []any{map[string]any{"text": "Ready", "width": 5, "flags": 0}}}},
		}})
	}
	for {
		var request struct{ ID, Op string }
		if err := decoder.Decode(&request); err != nil {
			return err
		}
		if request.Op == "open" && burst < 0 {
			message := map[string]any{"type": "process_group"}
			switch burst {
			case -2:
				message["pid"] = 1
			case -3:
				message["pid"] = -1
			case -4:
				message["pid"] = os.Getpid()
			}
			if err := encoder.Encode(message); err != nil {
				return err
			}
		}
		if request.Op == "reply" {
			for index := range burst {
				if err := data(fmt.Sprintf("frame-%d", index)); err != nil {
					return err
				}
			}
		}
		if err := encoder.Encode(map[string]any{"type": "response", "id": request.ID, "ok": true}); err != nil {
			return err
		}
		if request.Op == "close" {
			return nil
		}
		if request.Op == "open" {
			if err := data("\x1b[6n"); err != nil {
				return err
			}
		}
	}
}

func rawFixture() (err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if os.Getenv("HOME") != filepath.Join(filepath.Dir(cwd), "home") ||
		os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" ||
		!term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("synthetic Go fixture requires its isolated owned terminal")
	}
	saved, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, term.Restore(int(os.Stdin.Fd()), saved)) }()
	if _, err := io.WriteString(os.Stdout, "\x1b[?1000h\x1b[?1006hREADY\r\n"); err != nil {
		return err
	}
	var received []byte
	for {
		var data [256]byte
		count, err := os.Stdin.Read(data[:])
		if err != nil {
			return err
		}
		received = append(received, data[:count]...)
		if len(received) > 65536 {
			return fmt.Errorf("synthetic input exceeded its bound")
		}
		if _, err := fmt.Fprintf(os.Stdout, "INPUT:%s\r\n", hex.EncodeToString(received)); err != nil {
			return err
		}
		for _, value := range data[:count] {
			if value == '?' {
				columns, rows, err := term.GetSize(int(os.Stdin.Fd()))
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(os.Stdout, "SIZE:%dx%d\r\n", columns, rows); err != nil {
					return err
				}
			}
		}
	}
}
