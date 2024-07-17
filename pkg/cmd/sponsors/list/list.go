package listcmd

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type Sponsor string

type SponsorLister interface {
	ListSponsors(User) ([]Sponsor, error)
}

type SponsorListRenderer interface {
	Render([]Sponsor) error
}

type User string

type Options struct {
	SponsorLister       SponsorLister
	SponsorListRenderer SponsorListRenderer

	User User
}

func NewCmdList(f *cmdutil.Factory, runF func(Options) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sponsors for a user",
		Example: heredoc.Doc(`
			List sponsors of a user
			$ gh sponsors list <user>
		`),
		Aliases: []string{"ls"},
		Args:    cmdutil.ExactArgs(1, "must specify a user"),
		RunE: func(cmd *cobra.Command, args []string) error {
			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts := Options{
				SponsorLister: GQLSponsorClient{
					Hostname:  "github.com",
					APIClient: api.NewClientFromHTTP(httpClient),
				},
				SponsorListRenderer: ConsoleListRenderer{
					IO: f.IOStreams,
				},
				User: User(args[0]),
			}

			if runF != nil {
				return runF(opts)
			}
			return ListRun(opts)
		},
	}

	return cmd
}

func ListRun(opts Options) error {
	sponsors, err := opts.SponsorLister.ListSponsors(opts.User)
	if err != nil {
		return fmt.Errorf("sponsor list: %v", err)
	}

	return opts.SponsorListRenderer.Render(sponsors)
}
