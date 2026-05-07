package prompter

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type surveyEscapeTestReader struct {
	chunks [][]byte
}

func (r *surveyEscapeTestReader) Fd() uintptr {
	return 0
}

func (r *surveyEscapeTestReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks[0] = r.chunks[0][n:]
	if len(r.chunks[0]) == 0 {
		r.chunks = r.chunks[1:]
	}
	return n, nil
}

type surveyEscapeTestWriter struct {
	*bytes.Buffer
}

func (w surveyEscapeTestWriter) Fd() uintptr {
	return 0
}

func TestSurveyEscapeReaderPreservesAcceptedSurveySequences(t *testing.T) {
	input := []byte("a\x1b[A\x1b[B\x1b[C\x1b[D\x1b[F\x1b[H\x1b[3~\x1b[1;1Rz")
	require.Equal(t, input, normalizeSurveyEscapes(input))
}

func TestSurveyEscapeReaderNormalizesRejectedEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []byte
	}{
		{name: "meta printable key", input: "\x1bf", want: []byte{surveyIgnoreKey}},
		{name: "meta backspace", input: "\x1b\x7f", want: []byte{surveyIgnoreKey}},
		{name: "double escape", input: "\x1b\x1b", want: []byte{surveyIgnoreKey}},
		{name: "modified arrow", input: "\x1b[1;3D", want: []byte{surveyIgnoreKey}},
		{name: "kitty modified arrow", input: "\x1b[1;3:1D", want: []byte{surveyIgnoreKey}},
		{name: "modified delete", input: "\x1b[3;3~", want: []byte{surveyIgnoreKey}},
		{name: "csi u", input: "\x1b[127;3u", want: []byte{surveyIgnoreKey}},
		{name: "modifyOtherKeys", input: "\x1b[27;3;127~", want: []byte{surveyIgnoreKey}},
		{name: "interrupted csi", input: "\x1b[00\x1bX", want: []byte{surveyIgnoreKey, 'X'}},
		{name: "trailing escape remains escape", input: "\x1b", want: []byte{'\x1b'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeSurveyEscapes([]byte(tt.input)))
		})
	}
}

func TestSurveyEscapeReaderDoesNotWaitForSplitEscape(t *testing.T) {
	reader := newSurveyEscapeReader(&surveyEscapeTestReader{
		chunks: [][]byte{{'\x1b'}, {'f'}},
	})

	buf := make([]byte, 8)
	n, err := reader.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{'\x1b'}, buf[:n])

	n, err = reader.Read(buf)
	require.NoError(t, err)
	require.Equal(t, []byte{'f'}, buf[:n])
}

func TestSurveyEscapeReaderBuffersSplitControlSequences(t *testing.T) {
	tests := []struct {
		name   string
		chunks [][]byte
		want   []byte
	}{
		{
			name:   "split arrow",
			chunks: [][]byte{{'\x1b', '['}, {'A'}},
			want:   []byte("\x1b[A"),
		},
		{
			name:   "split delete",
			chunks: [][]byte{{'\x1b', '[', '3'}, {'~'}},
			want:   []byte("\x1b[3~"),
		},
		{
			name:   "split modified arrow",
			chunks: [][]byte{{'\x1b', '[', '1', ';', '3'}, {'D'}},
			want:   []byte{surveyIgnoreKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newSurveyEscapeReader(&surveyEscapeTestReader{chunks: tt.chunks})

			buf := make([]byte, 8)
			n, err := reader.Read(buf)
			require.NoError(t, err)
			require.Equal(t, tt.want, buf[:n])
		})
	}
}

func TestSurveyEscapeReaderAvoidsSurveyEscapeErrors(t *testing.T) {
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
			requireNoRejectedSurveyEscapes(t, normalizeSurveyEscapes(tt.input))
		})
	}
}

func FuzzSurveyEscapeReader(f *testing.F) {
	seeds := [][]byte{
		[]byte("abc\x1bf\r"),
		[]byte("abc\x1bb\r"),
		[]byte("abc\x1b\x7f\r"),
		[]byte("abc\x1b\x1b\r"),
		[]byte("abc\x1bq\r"),
		[]byte("abc\x1b[1;3D\r"),
		[]byte("abc\x1b[1;3:1D\r"),
		[]byte("abc\x1b[3;3~\r"),
		[]byte("abc\x1b[127;3u\r"),
		[]byte("abc\x1b[27;3;127~\r"),
		[]byte("abc\x1b[57426;3u\r"),
		[]byte("abc\x1b[00\x1bX\r"),
		[]byte("\x1b[3;5~"),
		[]byte("\x1b]52;c;AAAA\x07"),
		[]byte("\x1bP$qm\x1b\\"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		requireNoRejectedSurveyEscapes(t, normalizeSurveyEscapes(input))
		requireNoRejectedSurveyEscapes(t, normalizeSurveyEscapes(expandSurveyEscapeFuzzInput(input)))
	})
}

func expandSurveyEscapeFuzzInput(input []byte) []byte {
	if len(input) == 0 {
		return input
	}

	var expanded bytes.Buffer
	for index := 0; index < len(input); index++ {
		b := input[index]
		switch b % 12 {
		case 0:
			expanded.WriteByte('\x1b')
			expanded.WriteByte(byte('!' + b%94))
		case 1:
			expanded.WriteString("\x1b[")
			expanded.WriteByte(byte('0' + b%10))
			expanded.WriteByte(';')
			expanded.WriteByte(byte('0' + byteAt(input, index+1)%10))
			expanded.WriteByte(finalEscapeByte(byteAt(input, index+2)))
		case 2:
			expanded.WriteString("\x1b[")
			expanded.WriteByte(byte('0' + b%10))
			expanded.WriteByte(';')
			expanded.WriteByte(byte('0' + byteAt(input, index+1)%10))
			expanded.WriteByte(':')
			expanded.WriteByte(byte('0' + byteAt(input, index+2)%10))
			expanded.WriteByte(finalEscapeByte(byteAt(input, index+3)))
		case 3:
			expanded.WriteString("\x1b[27;")
			expanded.WriteByte(byte('0' + b%10))
			expanded.WriteByte(';')
			expanded.WriteByte(byte('0' + byteAt(input, index+1)%10))
			expanded.WriteByte(byte('0' + byteAt(input, index+2)%10))
			expanded.WriteByte(byte('0' + byteAt(input, index+3)%10))
			expanded.WriteByte('~')
		case 4:
			expanded.WriteString("\x1bO")
			expanded.WriteByte(finalEscapeByte(b))
		case 5:
			expanded.WriteString("\x1b]")
			expanded.WriteByte(byte('0' + b%10))
			expanded.WriteByte(';')
			expanded.WriteByte(byte('a' + b%26))
			expanded.WriteByte('\a')
		case 6:
			expanded.WriteString("\x1bP")
			expanded.WriteByte(byte(' ' + b%95))
			expanded.WriteString("\x1b\\")
		default:
			expanded.WriteByte(b)
		}
	}
	return expanded.Bytes()
}

func byteAt(input []byte, index int) byte {
	if index < 0 || index >= len(input) {
		return 0
	}
	return input[index]
}

func finalEscapeByte(b byte) byte {
	finals := []byte{'A', 'B', 'C', 'D', 'F', 'H', '~', 'u', 'm', 'X'}
	return finals[int(b)%len(finals)]
}

func readSurveyEscapeReader(t *testing.T, input []byte) []byte {
	t.Helper()

	reader := newSurveyEscapeReader(&surveyEscapeTestReader{chunks: [][]byte{input}})
	buf := make([]byte, 8)
	readLimit := len(input)*8 + 256
	var output bytes.Buffer
	for output.Len() < readLimit {
		n, err := reader.Read(buf)
		output.Write(buf[:n])
		if err == io.EOF {
			return output.Bytes()
		}
		require.NoError(t, err)
	}

	t.Fatalf("Survey escape reader did not reach EOF within %d bytes", readLimit)
	return nil
}

func requireNoRejectedSurveyEscapes(t *testing.T, input []byte) {
	t.Helper()

	for index := 0; index < len(input); {
		if input[index] != '\x1b' {
			index++
			continue
		}
		if index+1 >= len(input) {
			return
		}
		next := input[index+1]
		require.Truef(t, next == '[' || next == 'O', "normalizer emitted rejected escape sequence: %q", input[index:index+2])

		end := controlSequenceEnd(input, index+2)
		require.NotEqualf(t, -1, end, "normalizer emitted incomplete control sequence: %q", input[index:])

		sequence := input[index : end+1]
		require.Truef(t, surveyAcceptsEscapeSequence(sequence), "normalizer emitted control sequence Survey rejects: %q", sequence)
		index = end + 1
	}
}
