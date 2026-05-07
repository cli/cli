package prompter

import (
	"bytes"

	ghPrompter "github.com/cli/go-gh/v2/pkg/prompter"
)

const surveyIgnoreKey = '\x00'

const maxBufferedEscapeBytes = 32

type surveyEscapeReader struct {
	input         ghPrompter.FileReader
	pending       []byte
	partialEscape []byte
}

func newSurveyEscapeReader(input ghPrompter.FileReader) ghPrompter.FileReader {
	return &surveyEscapeReader{input: input}
}

func (r *surveyEscapeReader) Fd() uintptr {
	return r.input.Fd()
}

func (r *surveyEscapeReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		return n, nil
	}

	for {
		raw := make([]byte, len(p))
		n, err := r.input.Read(raw)
		if n == 0 {
			if err != nil && len(r.partialEscape) > 0 {
				normalized := normalizeSurveyEscapes(r.partialEscape)
				r.partialEscape = nil
				written := copy(p, normalized)
				r.pending = append(r.pending, normalized[written:]...)
				return written, nil
			}
			return 0, err
		}

		input := append(append([]byte(nil), r.partialEscape...), raw[:n]...)
		r.partialEscape = nil

		normalized, partialEscape := normalizeSurveyEscapesForRead(input)
		r.partialEscape = partialEscape
		if err != nil && len(r.partialEscape) > 0 {
			normalized = append(normalized, normalizeSurveyEscapes(r.partialEscape)...)
			r.partialEscape = nil
		}
		if len(normalized) == 0 && err == nil {
			continue
		}

		written := copy(p, normalized)
		r.pending = append(r.pending, normalized[written:]...)
		return written, err
	}
}

func normalizeSurveyEscapes(input []byte) []byte {
	normalized, partialEscape := normalizeSurveyEscapesInternal(input, false)
	if len(partialEscape) > 0 {
		normalized = append(normalized, surveyIgnoreKey)
	}
	return normalized
}

func normalizeSurveyEscapesForRead(input []byte) ([]byte, []byte) {
	return normalizeSurveyEscapesInternal(input, true)
}

func normalizeSurveyEscapesInternal(input []byte, bufferPartial bool) ([]byte, []byte) {
	var output bytes.Buffer
	for index := 0; index < len(input); {
		b := input[index]
		if b != '\x1b' {
			output.WriteByte(b)
			index++
			continue
		}

		if index+1 >= len(input) {
			output.WriteByte(b)
			index++
			continue
		}

		next := input[index+1]
		if next == '[' || next == 'O' {
			end := controlSequenceEnd(input, index+2)
			if end == -1 {
				partialEscape := input[index:]
				if bufferPartial && len(partialEscape) <= maxBufferedEscapeBytes {
					return output.Bytes(), append([]byte(nil), partialEscape...)
				}
				output.WriteByte(surveyIgnoreKey)
				index = len(input)
				continue
			}

			sequence := input[index : end+1]
			if surveyAcceptsEscapeSequence(sequence) {
				output.Write(sequence)
			} else {
				output.WriteByte(surveyIgnoreKey)
			}
			index = end + 1
			continue
		}

		output.WriteByte(surveyIgnoreKey)
		index += 2
	}
	return output.Bytes(), nil
}

func controlSequenceEnd(input []byte, start int) int {
	for index := start; index < len(input); index++ {
		b := input[index]
		if b == '\x1b' {
			return index
		}
		if isSurveyEscapeTerminator(b) {
			return index
		}
	}
	return -1
}

func isSurveyEscapeTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '~'
}

func surveyAcceptsEscapeSequence(sequence []byte) bool {
	if len(sequence) != 3 && len(sequence) != 4 {
		return surveyCursorPositionResponse(sequence)
	}

	keypad := sequence[1]
	if keypad != '[' && keypad != 'O' {
		return false
	}

	key := sequence[2]
	switch key {
	case 'A', 'B', 'C', 'D', 'F', 'H':
		return len(sequence) == 3
	case '3':
		return keypad == '[' && len(sequence) == 4 && sequence[3] == '~'
	default:
		return false
	}
}

func surveyCursorPositionResponse(sequence []byte) bool {
	if len(sequence) < 5 || sequence[0] != '\x1b' || sequence[1] != '[' || sequence[len(sequence)-1] != 'R' {
		return false
	}

	seenSemicolon := false
	for _, b := range sequence[2 : len(sequence)-1] {
		switch {
		case b >= '0' && b <= '9':
		case b == ';':
			seenSemicolon = true
		default:
			return false
		}
	}
	return seenSemicolon
}
