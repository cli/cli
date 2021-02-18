package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type FilterOptions struct {
	Visibility string // private, public
	Fork       bool
	Source     bool
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Limit int
	Owner string

	Visibility string
	Fork       bool
	Source     bool
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var (
		flagPublic  bool
		flagPrivate bool
		flagSource  bool
		flagFork    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Args:  cobra.MaximumNArgs(1),
		Short: "List repositories from a user or organization",
		RunE: func(c *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			if flagPrivate && flagPublic {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--public` or `--private`")}
			}
			if flagSource && flagFork {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--source` or `--fork`")}
			}

			if flagPrivate {
				opts.Visibility = "private"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			opts.Fork = flagFork
			opts.Source = flagSource

			if len(args) > 0 {
				opts.Owner = args[0]
			}

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories to list")
	cmd.Flags().BoolVar(&flagPrivate, "private", false, "Show only private repositories")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public repositories")
	cmd.Flags().BoolVar(&flagSource, "source", false, "Show only source repositories")
	cmd.Flags().BoolVar(&flagFork, "fork", false, "Show only forks")

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	isTerminal := opts.IO.IsStdoutTTY()

	owner := opts.Owner
	if owner == "" {
		owner, err = api.CurrentLoginName(apiClient, ghinstance.OverridableDefault())
		if err != nil {
			return err
		}
	}

	filter := FilterOptions{
		Visibility: opts.Visibility,
		Fork:       opts.Fork,
		Source:     opts.Source,
	}

	listResult, err := listRepos(apiClient, ghinstance.OverridableDefault(), opts.Limit, owner, filter)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	tp := utils.NewTablePrinter(opts.IO)

	notArchived := filter.Fork || filter.Source

	matchCount := len(listResult.Repositories)
	now := time.Now()

	for _, repo := range listResult.Repositories {
		if notArchived && repo.IsArchived {
			matchCount--
			listResult.TotalCount--
			continue
		}

		nameWithOwner := repo.NameWithOwner

		info := repo.Info()
		infoColor := cs.Gray
		visibility := "Public"

		if repo.IsPrivate {
			infoColor = cs.Yellow
			visibility = "Private"
		}

		description := repo.Description
		updatedAt := repo.UpdatedAt.Format(time.RFC3339)

		if tp.IsTTY() {
			tp.AddField(nameWithOwner, nil, cs.Bold)
			tp.AddField(text.ReplaceExcessiveWhitespace(description), nil, nil)
			tp.AddField(info, nil, infoColor)
			tp.AddField(utils.FuzzyAgoAbbr(now, repo.UpdatedAt), nil, nil)
		} else {
			tp.AddField(nameWithOwner, nil, nil)
			tp.AddField(text.ReplaceExcessiveWhitespace(description), nil, nil)
			tp.AddField(visibility, nil, nil)
			tp.AddField(updatedAt, nil, nil)
		}
		tp.EndRow()
	}

	if isTerminal {
		hasFilters := filter.Visibility != "" || filter.Fork || filter.Source
		title := listHeader(owner, matchCount, listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	return tp.Render()
}

func listHeader(owner string, matchCount, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return "No results match your search"
		}
		return "There are no repositories in @" + owner
	}

	return fmt.Sprintf("Showing %d of %d repositories in @%s", matchCount, totalMatchCount, owner)
}

type Repository struct {
	NameWithOwner string
	Description   string
	IsFork        bool
	IsPrivate     bool
	IsArchived    bool
	UpdatedAt     time.Time
}

func (r Repository) Info() string {
	var info string
	var tags []string

	if r.IsPrivate {
		tags = append(tags, "private")
	}
	if r.IsFork {
		tags = append(tags, "fork")
	}
	if r.IsArchived {
		tags = append(tags, "archived")
	}

	if len(tags) > 0 {
		tags[0] = strings.Title(tags[0])
		info = strings.Join(tags, ", ")
	}

	return info
}
