package sshconfig

import (
	"io"
	"strings"
	"text/template"

	"github.com/MakeNowJust/heredoc"
)

// Config contains values needed to write an OpenSSH host configuration for a
// single codespace. For example:
//
// Host {{Name}}.{{EscapedRef}
//
//	User {{SSHUser}
//	ProxyCommand {{GHExec}} cs ssh -c {{Name}} --stdio
//
// EscapedRef is included in the name to help distinguish between codespaces
// when tab-completing ssh hostnames. '/' characters in EscapedRef are flattened
// to '-' to prevent problems with tab completion or when the hostname appears
// in ControlMaster socket paths.
type Config struct {
	Name                      string // the codespace name, passed to `ssh -c`
	Ref                       string // the currently checked-out branch
	SSHUser                   string // the remote ssh username
	GHExec                    string // path used for invoking the current `gh` binary
	AutomaticIdentityFilePath string // path used for automatic private key `gh cs ssh` would generate
}

var t = template.Must(
	template.New("ssh_config").Parse(heredoc.Doc(`
		Host cs.{{.Name}}.{{.EscapedRef}}
			User {{.SSHUser}}
			ProxyCommand {{.GHExec}} cs ssh -c {{.Name}} --stdio -- -i {{.AutomaticIdentityFilePath}}
			UserKnownHostsFile=/dev/null
			StrictHostKeyChecking no
			LogLevel quiet
			ControlMaster auto
			IdentityFile {{.AutomaticIdentityFilePath}}

	`)),
)

// ExcapedRef flattens '/' characters in the ref to '-' to prevent problems
// with tab completion or when the hostname appears in ControlMaster socket
// paths.
func (c Config) EscapedRef() string {
	return strings.ReplaceAll(c.Ref, "/", "-")
}

// Print writes the configuration to the given writer (e.g. a file or
// os.Stdout).
func (c Config) Print(wr io.Writer) error {
	return t.Execute(wr, c)
}
