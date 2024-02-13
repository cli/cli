package attestation

import (
	"github.com/github/gh-attestation/cmd/app"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdAttestation(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:     "attestation",
		Short:   "attestations",
		// Aliases: []string{"cs"},
		// GroupID: "core",
	}

	version := "test"
	date := "test"
	shortSHA := "test"

	root.AddCommand(app.NewDownloadCmd(version, date))
	root.AddCommand(app.NewVerifyCmd(version, date))
	root.AddCommand(app.NewVersionCmd(version, date, shortSHA))
	root.AddCommand(app.NewTUFRootVerifyCmd())

	return root
}
