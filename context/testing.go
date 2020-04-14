package context

import (
	"io"
	"reflect"
	"testing"
)

func StubBackupConfig() func() {
	orig := BackupConfigFile
	BackupConfigFile = func(_ string) error {
		return nil
	}

	return func() {
		BackupConfigFile = orig
	}
}

func StubWriteConfig(w io.Writer) func() {
	orig := WriteConfigFile
	WriteConfigFile = func(fn string, data []byte) error {
		_, err := w.Write(data)
		return err
	}
	return func() {
		WriteConfigFile = orig
	}
}

func StubConfig(content string) func() {
	orig := ReadConfigFile
	ReadConfigFile = func(fn string) ([]byte, error) {
		return []byte(content), nil
	}
	return func() {
		ReadConfigFile = orig
	}
}

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}
