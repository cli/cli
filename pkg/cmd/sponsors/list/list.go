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

type UserPrompter interface {
	PromptForUser() (User, error)
}

type Options struct {
	SponsorLister       SponsorLister
	SponsorListRenderer SponsorListRenderer

	UserPrompter UserPrompter

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
		Args:    cmdutil.MaximumArgs(1, "too many arguments"),
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
			}

			// I don't love this. If we _know_ we don't have a user then we should just
			// figure it out now instead of passing it onto the ListRun function.
			//
			// However, I also don't really want to prompt in this function because it's
			// hard to test cobra lifecycle code. I think maybe it indicates a missing abstraction.
			//
			// Food for thought!
			if len(args) > 0 {
				opts.User = User(args[0])
			} else {
				opts.UserPrompter = ConsoleUserPrompter{
					Prompter: f.Prompter,
				}
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
	if opts.User == "" {
		user, err := opts.UserPrompter.PromptForUser()
		if err != nil {
			return fmt.Errorf("prompt for user: %v", err)
		}
		opts.User = user
	}

	sponsors, err := opts.SponsorLister.ListSponsors(opts.User)
	if err != nil {
		return fmt.Errorf("sponsor list: %v", err)
	}

	return opts.SponsorListRenderer.Render(sponsors)
}
