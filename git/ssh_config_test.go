package git

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
)

func Test_sshParser_read(t *testing.T) {
	testFiles := map[string]string{
		"/etc/ssh/config": heredoc.Doc(`
			Include sites/*
		`),
		"/etc/ssh/sites/cfg1": heredoc.Doc(`
			Host s1
			Hostname=site1.net
		`),
		"/etc/ssh/sites/cfg2": heredoc.Doc(`
			Host s2
			Hostname = site2.net
		`),
		"HOME/.ssh/config": heredoc.Doc(`
			Host *
			Host gh gittyhubby
				Hostname github.com
				#Hostname example.com
			Host ex
			  Include ex_config/*
		`),
		"HOME/.ssh/ex_config/ex_cfg": heredoc.Doc(`
			Hostname example.com
		`),
	}
	globResults := map[string][]string{
		"/etc/ssh/sites/*":      {"/etc/ssh/sites/cfg1", "/etc/ssh/sites/cfg2"},
		"HOME/.ssh/ex_config/*": {"HOME/.ssh/ex_config/ex_cfg"},
	}

	p := &sshParser{
		homeDir: "HOME",
		open: func(s string) (io.Reader, error) {
			if contents, ok := testFiles[filepath.ToSlash(s)]; ok {
				return bytes.NewBufferString(contents), nil
			} else {
				return nil, fmt.Errorf("no test file stub found: %q", s)
			}
		},
		glob: func(p string) ([]string, error) {
			if results, ok := globResults[filepath.ToSlash(p)]; ok {
				return results, nil
			} else {
				return nil, fmt.Errorf("no glob stubs found: %q", p)
			}
		},
	}

	if err := p.read("/etc/ssh/config"); err != nil {
		t.Fatalf("read(global config) = %v", err)
	}
	if err := p.read("HOME/.ssh/config"); err != nil {
		t.Fatalf("read(user config) = %v", err)
	}

	if got := p.aliasMap["gh"]; got != "github.com" {
		t.Errorf("expected alias %q to expand to %q, got %q", "gh", "github.com", got)
	}
	if got := p.aliasMap["gittyhubby"]; got != "github.com" {
		t.Errorf("expected alias %q to expand to %q, got %q", "gittyhubby", "github.com", got)
	}
	if got := p.aliasMap["example.com"]; got != "" {
		t.Errorf("expected alias %q to expand to %q, got %q", "example.com", "", got)
	}
	if got := p.aliasMap["ex"]; got != "example.com" {
		t.Errorf("expected alias %q to expand to %q, got %q", "ex", "example.com", got)
	}
	if got := p.aliasMap["s1"]; got != "site1.net" {
		t.Errorf("expected alias %q to expand to %q, got %q", "s1", "site1.net", got)
	}
}

func Test_sshParser_absolutePath(t *testing.T) {
	dir := "HOME"
	p := &sshParser{homeDir: dir}

	tests := map[string]struct {
		parentFile string
		arg        string
		want       string
		wantErr    bool
	}{
		"absolute path": {
			parentFile: "/etc/ssh/ssh_config",
			arg:        "/etc/ssh/config",
			want:       "/etc/ssh/config",
		},
		"system relative path": {
			parentFile: "/etc/ssh/config",
			arg:        "configs/*.conf",
			want:       filepath.Join("/etc", "ssh", "configs", "*.conf"),
		},
		"user relative path": {
			parentFile: filepath.Join(dir, ".ssh", "ssh_config"),
			arg:        "configs/*.conf",
			want:       filepath.Join(dir, ".ssh", "configs/*.conf"),
		},
		"shell-like ~ rerefence": {
			parentFile: filepath.Join(dir, ".ssh", "ssh_config"),
			arg:        "~/.ssh/*.conf",
			want:       filepath.Join(dir, ".ssh", "*.conf"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := p.absolutePath(tt.parentFile, tt.arg); got != tt.want {
				t.Errorf("absolutePath(): %q, wants %q", got, tt.want)
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
