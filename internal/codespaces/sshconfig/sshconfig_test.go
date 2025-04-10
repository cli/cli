package sshconfig

import (
	"bytes"
	"errors"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/google/go-cmp/cmp"
)

func Test_Print(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wanted  string
		wantErr error
	}{
		{
			name: "valid config",
			config: Config{
				Name:                      "test-codespace",
				Ref:                       "franciscoj/fix/whatever",
				SSHUser:                   "codespace",
				GHExec:                    "/usr/local/bin/gh",
				AutomaticIdentityFilePath: "/path/to/identity/file",
			},
			wanted: heredoc.Doc(`
				Host cs.test-codespace.franciscoj-fix-whatever
					User codespace
					ProxyCommand /usr/local/bin/gh cs ssh -c test-codespace --stdio -- -i /path/to/identity/file
					UserKnownHostsFile=/dev/null
					StrictHostKeyChecking no
					LogLevel quiet
					ControlMaster auto
					IdentityFile /path/to/identity/file

			`),
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			err := tt.config.Print(buffer)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Print() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wanted, buffer.String()); diff != "" {
				t.Errorf("Print() got = \n%v, wanted = \n%v, diff = \n%s", buffer.String(), tt.wanted, diff)
			}
		})
	}
}
