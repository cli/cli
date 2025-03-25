package list

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/shurcooL/graphql"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

const defaultListLimit = 30

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)

	Username string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list <user>",
		Short: "List sponsors",
		Long: heredoc.Doc(`
			List sponsors of a given user.
		`),
		Aliases: []string{"ls"},
		Args:    cmdutil.ExactArgs(1, "must specify username"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// This is for consistency with the other commands, but the check seems
			// irrelevant since we have already used the cobra.ExactArgs.
			if len(args) > 0 {
				opts.Username = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	username := opts.Username

	hostname, _ := cfg.Authentication().DefaultHost()
	sponsors, err := listSponsors(client, hostname, username, defaultListLimit)
	if err != nil {
		return err
	}

	if len(sponsors) == 0 {
		return cmdutil.NewNoResultsError("no sponsor found")
	}

	headers := []string{"Sponsor"}
	table := tableprinter.New(opts.IO, tableprinter.WithHeader(headers...))
	for _, sponsor := range sponsors {
		table.AddField(sponsor)
		table.EndRow()
	}

	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

func listSponsors(httpClient *http.Client, hostname string, username string, limit uint) ([]string, error) {
	var query struct {
		User struct {
			Sponsors struct {
				Edges []struct {
					Node struct {
						User struct {
							Login graphql.String
						} `graphql:"... on User"`
						Org struct {
							Login graphql.String
						} `graphql:"... on Organization"`
					}
				}
			} `graphql:"sponsors(first: $limit, orderBy: { direction: ASC, field: LOGIN })"`
		} `graphql:"user(login: $login)"`
	}

	client := api.NewClientFromHTTP(httpClient)

	variables := map[string]any{
		"login": graphql.String(username),
		"limit": graphql.Int(limit),
	}

	err := client.Query(hostname, "UserSponsorList", &query, variables)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(query.User.Sponsors.Edges))
	for _, edge := range query.User.Sponsors.Edges {
		if edge.Node.User.Login != "" {
			result = append(result, string(edge.Node.User.Login))
		} else if edge.Node.Org.Login != "" {
			result = append(result, string(edge.Node.Org.Login))
		}
	}
	return result, nil
}
