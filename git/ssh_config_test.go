package git

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mitchellh/go-homedir"
)

// TODO: extract assertion helpers into a shared package
func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func createTempFile(t *testing.T, prefix string) *os.File {
	t.Helper()

	dir, err := homedir.Dir()
	if err != nil {
		t.Errorf("Could not find homedir: %s", err)
	}

	tempFile, err := ioutil.TempFile(filepath.Join(dir, ".ssh"), prefix)
	if err != nil {
		t.Errorf("Could create a temp file: %s", err)
	}

	t.Cleanup(func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	})

	return tempFile
}

func Test_parse(t *testing.T) {
	includedTempFile := createTempFile(t, "included")
	includedConfigFile := `
Host webapp
	HostName webapp.example.com
	`
	fmt.Fprint(includedTempFile, includedConfigFile)

	m := parse(
		"testdata/ssh_config1.conf",
		"testdata/ssh_config2.conf",
		"testdata/ssh_config3.conf",
	)

	eq(t, m["foo"], "example.com")
	eq(t, m["bar"], "%bar.net%")
	eq(t, m["nonexistent"], "")
}

func Test_absolutePaths(t *testing.T) {
	dir, err := homedir.Dir()
	if err != nil {
		t.Errorf("Could not find homedir: %s", err)
	}

	tests := map[string]struct {
		parentFile string
		Input      []string
		Want       []string
	}{
		"absolute path": {
			parentFile: "/etc/ssh/ssh_config",
			Input:      []string{"/etc/ssh/config"},
			Want:       []string{"/etc/ssh/config"},
		},
		"system relative path": {
			parentFile: "/etc/ssh/config",
			Input:      []string{"configs/*.conf"},
			Want:       []string{"/etc/ssh/configs/*.conf"},
		},
		"user relative path": {
			parentFile: filepath.Join(dir, ".ssh", "ssh_config"),
			Input:      []string{"configs/*.conf"},
			Want:       []string{filepath.Join(dir, ".ssh", "configs/*.conf")},
		},
		"shell-like ~ rerefence": {
			parentFile: filepath.Join(dir, ".ssh", "ssh_config"),
			Input:      []string{"~/.ssh/*.conf"},
			Want:       []string{filepath.Join(dir, ".ssh", "*.conf")},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			paths := absolutePaths(test.parentFile, test.Input)

			if len(paths) != len(test.Input) {
				t.Errorf("Expected %d, got %d", len(test.Input), len(paths))
			}

			for i, path := range paths {
				if path != test.Want[i] {
					t.Errorf("Expected %q, got %q", test.Want[i], path)
				}
			}
		})
	}
}

func Test_Translator(t *testing.T) {
	m := SSHAliasMap{
		"gh":         "github.com",
		"github.com": "ssh.github.com",
	}
	tr := m.Translator()

	cases := [][]string{
		{"ssh://gh/o/r", "ssh://github.com/o/r"},
		{"ssh://github.com/o/r", "ssh://github.com/o/r"},
		{"https://gh/o/r", "https://gh/o/r"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c[0])
		got := tr(u)
		if got.String() != c[1] {
			t.Errorf("%q: expected %q, got %q", c[0], c[1], got)
		}
	}
}
