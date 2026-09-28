package execution

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/term"
)

func TestMain(tests *testing.M) {
	mode := os.Getenv("CLI_EXERCISE_TEST_FIXTURE")
	if mode == "workflow" {
		code, err := terminalFixture()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			code = 3
		}
		os.Exit(code)
	}
	os.Exit(tests.Run())
}

func terminalFixture() (code int, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return 3, err
	}
	if os.Getenv("HOME") != filepath.Join(filepath.Dir(cwd), "home") ||
		os.Getenv("GH_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" ||
		!term.IsTerminal(int(os.Stdin.Fd())) {
		return 3, fmt.Errorf("synthetic Go fixture requires its isolated owned terminal")
	}
	saved, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return 3, err
	}
	defer func() { err = errors.Join(err, term.Restore(int(os.Stdin.Fd()), saved)) }()
	return workflowFixture(os.Stdin, os.Stdout)
}

func workflowFixture(input io.Reader, output io.Writer) (int, error) {
	const heading = "\x1b[2J\x1b[HSYNTHETIC GO CLI WORKFLOW\r\n"
	if _, err := io.WriteString(output, heading+"Name: "); err != nil {
		return 3, err
	}
	name := ""
	menu, selected := false, 0
	choices := []string{"Red", "Blue"}
	showMenu := func() error {
		if _, err := io.WriteString(output, heading+"Choose a color\r\n"); err != nil {
			return err
		}
		for index, choice := range choices {
			marker := " "
			if index == selected {
				marker = ">"
			}
			if _, err := fmt.Fprintf(output, "%s %s\r\n", marker, choice); err != nil {
				return err
			}
		}
		return nil
	}
	var pending []byte
	for {
		var data [128]byte
		count, err := input.Read(data[:])
		if err != nil {
			return 3, err
		}
		pending = append(pending, data[:count]...)
		for len(pending) > 0 {
			character := pending[0]
			if character == '\x03' {
				_, err := io.WriteString(output, "\r\nFixture interrupted.\r\n")
				return 2, err
			}
			if menu && character == '\x1b' {
				if len(pending) < 3 {
					break
				}
				if (pending[1] == '[' || pending[1] == 'O') && (pending[2] == 'A' || pending[2] == 'B') {
					selected = (selected + 1) % len(choices)
					pending = pending[3:]
					if err := showMenu(); err != nil {
						return 3, err
					}
					continue
				}
				return 4, fmt.Errorf("unsupported synthetic menu input")
			}
			pending = pending[1:]
			if character == '\r' || character == '\n' {
				if menu {
					_, err := fmt.Fprintf(output, heading+"Hello, %s.\r\n\x1b[1;34mSelected: %s.\x1b[0m\r\nComplete.\r\n", name, choices[selected])
					return 0, err
				}
				if name == "" {
					if _, err := io.WriteString(output, "\r\nName cannot be blank.\r\nName: "); err != nil {
						return 3, err
					}
					continue
				}
				menu = true
				if err := showMenu(); err != nil {
					return 3, err
				}
			} else if !menu && (character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == ' ') {
				name += string(character)
				if _, err := output.Write([]byte{character}); err != nil {
					return 3, err
				}
			} else {
				return 4, fmt.Errorf("unsupported synthetic fixture input")
			}
		}
	}
}
