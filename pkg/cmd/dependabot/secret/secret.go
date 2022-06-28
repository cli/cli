package secret

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewCmdSecret(f *cmdutil.Factory) *cobra.Command {
	cs := f.IOStreams.ColorScheme()

	cmd := &cobra.Command{
		Use:    "secret",
		Short:  "Learn about managing Dependabot secrets",
		Long:   secretExplainer(cs),
		Hidden: true,
		Example: heredoc.Doc(`
			# Set repository-level secret for Dependabot
			$ gh secret set MYSECRET --app dependabot

			# List org-level secrets for Dependabot
			$ gh secret list --org myOrg --app dependabot

			# Delete repository-level secret for Dependabot
			$ gh secret delete MYSECRET --app dependabot
		`),
	}

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}

func secretExplainer(cs *iostreams.ColorScheme) string {
	header := cs.Bold(heredoc.Doc(`
		Dependabot secrets can be managed by running "gh secret" subcommands
		and passing the "--app dependabot" option.
	`))

	return heredoc.Docf(`
			%s
			gh secret delete:      Delete secrets
			gh secret list:        List secrets
			gh secret set:         Create or update secrets

			To see more help, run 'gh help run secret <subcommand>'`, header)
}
