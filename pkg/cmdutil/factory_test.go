package cmdutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_executable(t *testing.T) {
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testExeName := filepath.Base(testExe)

	// Create 3 extra PATH entries that each contain an executable with the same name as the running test
	// process. The first is a symlink, but to an unrelated executable, the second is a symlink to our test
	// process and thus represents the result we want, and the third one is an unrelated executable.
	dir := t.TempDir()
	bin1 := filepath.Join(dir, "bin1")
	bin1Exe := filepath.Join(bin1, testExeName)
	bin2 := filepath.Join(dir, "bin2")
	bin2Exe := filepath.Join(bin2, testExeName)
	bin3 := filepath.Join(dir, "bin3")
	bin3Exe := filepath.Join(bin3, testExeName)

	if err := os.MkdirAll(bin1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin3, 0755); err != nil {
		t.Fatal(err)
	}
	if f, err := os.OpenFile(bin3Exe, os.O_CREATE, 0755); err == nil {
		f.Close()
	} else {
		t.Fatal(err)
	}
	if err := os.Symlink(testExe, bin2Exe); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin3Exe, bin1Exe); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", strings.Join([]string{bin1, bin2, bin3, oldPath}, string(os.PathListSeparator)))

	if got := executable(""); got != bin2Exe {
		t.Errorf("executable() = %q, want %q", got, bin2Exe)
	}
}

func Test_executable_relative(t *testing.T) {
	testExe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testExeName := filepath.Base(testExe)

	// Create 3 extra PATH entries that each contain an executable with the same name as the running test
	// process. The first is a relative symlink, but to an unrelated executable, the second is a relative
	// symlink to our test process and thus represents the result we want, and the third one is an unrelated
	// executable.
	dir := t.TempDir()
	bin1 := filepath.Join(dir, "bin1")
	bin1Exe := filepath.Join(bin1, testExeName)
	bin2 := filepath.Join(dir, "bin2")
	bin2Exe := filepath.Join(bin2, testExeName)
	bin3 := filepath.Join(dir, "bin3")
	bin3Exe := filepath.Join(bin3, testExeName)

	if err := os.MkdirAll(bin1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin3, 0755); err != nil {
		t.Fatal(err)
	}

	if f, err := os.OpenFile(bin3Exe, os.O_CREATE, 0755); err == nil {
		f.Close()
	} else {
		t.Fatal(err)
	}
	bin2Rel, err := filepath.Rel(bin2, testExe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin2Rel, bin2Exe); err != nil {
		t.Fatal(err)
	}
	bin1Rel, err := filepath.Rel(bin1, bin3Exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bin1Rel, bin1Exe); err != nil {
		t.Fatal(err)
	}

	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", strings.Join([]string{bin1, bin2, bin3, oldPath}, string(os.PathListSeparator)))

	if got := executable(""); got != bin2Exe {
		t.Errorf("executable() = %q, want %q", got, bin2Exe)
	}
}

func Test_Executable_override(t *testing.T) {
	override := strings.Join([]string{"C:", "cygwin64", "home", "gh.exe"}, string(os.PathSeparator))
	t.Setenv("GH_PATH", override)
	f := Factory{}
	if got := f.Executable(); got != override {
		t.Errorf("executable() = %q, want %q", got, override)
	}
}
