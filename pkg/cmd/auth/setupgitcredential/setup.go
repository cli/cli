package login

import (
	"github.com/cli/cli/pkg/cmd/auth/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const tokenUser = "x-access-token"

type config interface {
	GetWithSource(string, string) (string, string, error)
}

type SetupCredentialOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config, error)
}

func NewCmdSetupCredential(f *cmdutil.Factory, runF func(*SetupCredentialOptions) error) *cobra.Command {
	opts := &SetupCredentialOptions{
		IO: f.IOStreams,
		Config: func() (config, error) {
			return f.Config()
		},
	}

	cmd := &cobra.Command{
		Use:   "setup-git-credential-helper",
		Args:  cobra.ExactArgs(1),
		Short: "Set the GH CLI as a git credential helper",
		RunE: func(_ *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}
			credentialFlow := &shared.GitCredentialFlow{}
			if err := credentialFlow.Setup("github.com", "", ""); err != nil {
				return err
			}
			return nil
		},
	}

	return cmd
}
