package attestation

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/attestation/download"
	"github.com/cli/cli/v2/pkg/cmd/attestation/inspect"
	"github.com/cli/cli/v2/pkg/cmd/attestation/trustedroot"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verify"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/spf13/cobra"
)

func NewCmdAttestation(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:     "attestation [subcommand]",
		Short:   "Work with artifact attestations",
		Aliases: []string{"at"},
		Long: heredoc.Doc(`
			Download and verify artifact attestations.
		`),
	}

	root.AddCommand(download.NewDownloadCmd(f, nil))
	root.AddCommand(inspect.NewInspectCmd(f, nil))
	root.AddCommand(verify.NewVerifyCmd(f, nil))
	root.AddCommand(trustedroot.NewTrustedRootCmd(f, nil))

	return root
}
