package list

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List deploy keys in a GitHub repository",
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	apiClient, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	deployKeys, err := repoKeys(apiClient, repo)
	if err != nil {
		return err
	}

	if len(deployKeys) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no deploy keys found in %s", ghrepo.FullName(repo)))
	}

	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	t := utils.NewTablePrinter(opts.IO)
	cs := opts.IO.ColorScheme()
	now := time.Now()

	for _, deployKey := range deployKeys {
		sshID := strconv.Itoa(deployKey.ID)
		t.AddField(sshID, nil, nil)
		t.AddField(deployKey.Title, nil, nil)

		sshType := "read-only"
		if !deployKey.ReadOnly {
			sshType = "read-write"
		}
		t.AddField(sshType, nil, nil)
		t.AddField(deployKey.Key, truncateMiddle, nil)

		createdAt := deployKey.CreatedAt.Format(time.RFC3339)
		if t.IsTTY() {
			createdAt = text.FuzzyAgoAbbr(now, deployKey.CreatedAt)
		}
		t.AddField(createdAt, nil, cs.Gray)
		t.EndRow()
	}

	return t.Render()
}

func truncateMiddle(maxWidth int, t string) string {
	if len(t) <= maxWidth {
		return t
	}

	ellipsis := "..."
	if maxWidth < len(ellipsis)+2 {
		return t[0:maxWidth]
	}

	halfWidth := (maxWidth - len(ellipsis)) / 2
	remainder := (maxWidth - len(ellipsis)) % 2
	return t[0:halfWidth+remainder] + ellipsis + t[len(t)-halfWidth:]
}
