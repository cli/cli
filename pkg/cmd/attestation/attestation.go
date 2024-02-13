package attestation

import (
	"github.com/github/gh-attestation/cmd/app"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

func NewCmdAttestation(io *iostreams.IOStreams, version, buildDate string) *cobra.Command {
	root := &cobra.Command{
		Use:     "attestation",
		Short:   "attestations",
	}

	root.AddCommand(app.NewDownloadCmd(version, buildDate))
	root.AddCommand(app.NewVerifyCmd(version, buildDate))
	root.AddCommand(app.NewTUFRootVerifyCmd())

	return root
}
