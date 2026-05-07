//go:build !windows

package prompter

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2/terminal"
)

func TestSurveyEscapeReaderAvoidsSurveyRuneReaderEscapeErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "option right", input: []byte("abc\x1bf\r")},
		{name: "option left", input: []byte("abc\x1bb\r")},
		{name: "option backspace", input: []byte("abc\x1b\x7f\r")},
		{name: "double escape", input: []byte("abc\x1b\x1b\r")},
		{name: "unknown meta key", input: []byte("abc\x1bq\r")},
		{name: "modified arrow sequence", input: []byte("abc\x1b[1;3D\r")},
		{name: "kitty modified arrow sequence", input: []byte("abc\x1b[1;3:1D\r")},
		{name: "modified delete sequence", input: []byte("abc\x1b[3;3~\r")},
		{name: "csi u option backspace", input: []byte("abc\x1b[127;3u\r")},
		{name: "modifyOtherKeys option backspace", input: []byte("abc\x1b[27;3;127~\r")},
		{name: "interrupted csi sequence", input: []byte("abc\x1b[00\x1bX\r")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireSurveyRuneReaderAccepts(t, tt.input)
		})
	}
}

func requireSurveyRuneReaderAccepts(t *testing.T, input []byte) {
	t.Helper()

	reader := newSurveyEscapeReader(&surveyEscapeTestReader{chunks: [][]byte{input}})
	runeReader := terminal.NewRuneReader(terminal.Stdio{
		In:  reader,
		Out: surveyEscapeTestWriter{Buffer: bytes.NewBuffer(nil)},
	})

	readLimit := len(input)*8 + 256
	for count := 0; count < readLimit; count++ {
		_, _, err := runeReader.ReadRune()
		if err == io.EOF {
			return
		}
		if err != nil {
			if strings.Contains(err.Error(), "unexpected escape sequence") {
				t.Fatalf("normalizer emitted a sequence Survey rejects: %v", err)
			}
			t.Fatalf("unexpected Survey read error: %v", err)
		}
	}

	t.Fatalf("Survey reader did not reach EOF within %d reads", readLimit)
}
