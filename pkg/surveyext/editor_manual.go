package surveyext

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	shellquote "github.com/kballard/go-shellquote"
)

type showable interface {
	Show()
}

func Edit(editorCommand, fn, initialValue string, stdin io.Reader, stdout io.Writer, stderr io.Writer, cursor showable) (string, error) {
	// prepare the temp file
	pattern := fn
	if pattern == "" {
		pattern = "survey*.txt"
	}
	f, err := ioutil.TempFile("", pattern)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())

	// write utf8 BOM header
	// The reason why we do this is because notepad.exe on Windows determines the
	// encoding of an "empty" text file by the locale, for example, GBK in China,
	// while golang string only handles utf8 well. However, a text file with utf8
	// BOM header is not considered "empty" on Windows, and the encoding will then
	// be determined utf8 by notepad.exe, instead of GBK or other encodings.
	if _, err := f.Write(bom); err != nil {
		return "", err
	}

	// write initial value
	if _, err := f.WriteString(initialValue); err != nil {
		return "", err
	}

	// close the fd to prevent the editor unable to save file
	if err := f.Close(); err != nil {
		return "", err
	}

	if editorCommand == "" {
		editorCommand = defaultEditor
	}
	args, err := shellquote.Split(editorCommand)
	if err != nil {
		return "", err
	}
	args = append(args, f.Name())

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if cursor != nil {
		cursor.Show()
	}

	// open the editor
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// raw is a BOM-unstripped UTF8 byte slice
	raw, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return "", err
	}

	// strip BOM header
	return string(bytes.TrimPrefix(raw, bom)), nil
}
