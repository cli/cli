package status

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	issueShared "github.com/cli/cli/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Exporter cmdutil.Exporter
}

func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of relevant issues",
		Args:  cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(opts)
			}
			return statusRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.IssueFields)

	return cmd
}

var defaultFields = []string{
	"number",
	"title",
	"url",
	"state",
	"updatedAt",
	"labels",
}

func statusRun(opts *StatusOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	currentUser, err := api.CurrentLoginName(apiClient, baseRepo.RepoHost())
	if err != nil {
		return err
	}

	options := api.IssueStatusOptions{
		Username: currentUser,
		Fields:   defaultFields,
	}
	if opts.Exporter != nil {
		options.Fields = opts.Exporter.Fields()
	}
	issuePayload, err := api.IssueStatus(apiClient, baseRepo, options)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		data := map[string]interface{}{
			"createdBy": issuePayload.Authored.Issues,
			"assigned":  issuePayload.Assigned.Issues,
			"mentioned": issuePayload.Mentioned.Issues,
		}
		return opts.Exporter.Write(opts.IO.Out, data, opts.IO.ColorEnabled())
	}

	out := opts.IO.Out

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "Relevant issues in %s\n", ghrepo.FullName(baseRepo))
	fmt.Fprintln(out, "")

	prShared.PrintHeader(opts.IO, "Issues assigned to you")
	if issuePayload.Assigned.TotalCount > 0 {
		issueShared.PrintIssues(opts.IO, "  ", issuePayload.Assigned.TotalCount, issuePayload.Assigned.Issues)
	} else {
		message := "  There are no issues assigned to you"
		prShared.PrintMessage(opts.IO, message)
	}
	fmt.Fprintln(out)

	prShared.PrintHeader(opts.IO, "Issues mentioning you")
	if issuePayload.Mentioned.TotalCount > 0 {
		issueShared.PrintIssues(opts.IO, "  ", issuePayload.Mentioned.TotalCount, issuePayload.Mentioned.Issues)
	} else {
		prShared.PrintMessage(opts.IO, "  There are no issues mentioning you")
	}
	fmt.Fprintln(out)

	prShared.PrintHeader(opts.IO, "Issues opened by you")
	if issuePayload.Authored.TotalCount > 0 {
		issueShared.PrintIssues(opts.IO, "  ", issuePayload.Authored.TotalCount, issuePayload.Authored.Issues)
	} else {
		prShared.PrintMessage(opts.IO, "  There are no issues opened by you")
	}
	fmt.Fprintln(out)

	return nil
}
