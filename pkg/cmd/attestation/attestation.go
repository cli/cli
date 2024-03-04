package attestation

import (
	"github.com/cli/cli/v2/pkg/cmd/attestation/download"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verify"
	"github.com/cli/cli/v2/pkg/cmdutil"
	
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewCmdAttestation(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:     "attestation [subcommand]",
		Short:   "Work with attestations",
		Aliases: []string{"at"},
		Long: heredoc.Docf(`
		Work with attestations that represent trusted metadata about artifacts and images.

		The %[1]sattestation%[1]s command and all subcommands support the following account types:
		* Free tier
		* Pro tier
		* Team tier
		* GHEC
		* GHEC EMU
	`, "`"),
	}

	root.AddCommand(download.NewDownloadCmd(f))
	root.AddCommand(verify.NewVerifyCmd(f))

	return root
}
