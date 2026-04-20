package itemlist

import (
	"fmt"
	"strconv"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/client"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type listOpts struct {
	limit    int
	owner    string
	number   int32
	query    string
	fields   []string
	fieldIDs []string
	exporter cmdutil.Exporter
}

type listConfig struct {
	io       *iostreams.IOStreams
	client   *queries.Client
	opts     listOpts
	detector fd.Detector
}

func NewCmdList(f *cmdutil.Factory, runF func(config listConfig) error) *cobra.Command {
	opts := listOpts{}
	listCmd := &cobra.Command{
		Short: "List the items in a project",
		Use:   "item-list [<number>]",
		Long: heredoc.Doc(`
			List the items in a project.

			If supported by the API host (github.com and GHES 3.20+), the --query option can
			be used to perform advanced search. For the full syntax, see:
			https://docs.github.com/en/issues/planning-and-tracking-with-projects/customizing-views-in-your-project/filtering-projects
		`),
		Example: heredoc.Doc(`
			# List the items in the current users's project "1"
			$ gh project item-list 1 --owner "@me"

			# List items assigned to a specific user
			$ gh project item-list 1 --owner "@me" --query "assignee:monalisa"

			# List open issues assigned to yourself
			$ gh project item-list 1 --owner "@me" --query "assignee:@me is:issue is:open"

			# List items with the "bug" label that are not done
			$ gh project item-list 1 --owner "@me" --query "label:bug -status:Done"

			# Show the "Status" and "Priority" field values as extra columns
			$ gh project item-list 1 --owner "@me" --field "Status" --field "Priority"
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.New(f)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				num, err := strconv.ParseInt(args[0], 10, 32)
				if err != nil {
					return cmdutil.FlagErrorf("invalid number: %v", args[0])
				}
				opts.number = int32(num)
			}

			if err := cmdutil.MutuallyExclusive(
				"only one of `--field` or `--field-id` may be used",
				len(opts.fields) > 0,
				len(opts.fieldIDs) > 0,
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"cannot use `--format` with `--field` or `--field-id`",
				opts.exporter != nil,
				len(opts.fields) > 0 || len(opts.fieldIDs) > 0,
			); err != nil {
				return err
			}

			config := listConfig{
				io:     f.IOStreams,
				client: client,
				opts:   opts,
			}

			// allow testing of the command without actually running it
			if runF != nil {
				return runF(config)
			}

			if opts.query != "" {
				httpClient, err := f.HttpClient()
				if err != nil {
					return err
				}
				cfg, err := f.Config()
				if err != nil {
					return err
				}
				host, _ := cfg.Authentication().DefaultHost()
				config.detector = fd.NewDetector(api.NewCachedHTTPClient(httpClient, time.Hour*24), host)
			}

			return runList(config)
		},
	}

	listCmd.Flags().StringVar(&opts.owner, "owner", "", "Login of the owner. Use \"@me\" for the current user")
	listCmd.Flags().StringVar(&opts.query, "query", "", `Filter items using the Projects filter syntax, e.g. "assignee:octocat -status:Done"`)
	listCmd.Flags().StringArrayVar(&opts.fields, "field", nil, "Name of a field to show as an extra column")
	listCmd.Flags().StringArrayVar(&opts.fieldIDs, "field-id", nil, "ID of a field to show as an extra column")
	cmdutil.AddFormatFlags(listCmd, &opts.exporter)
	listCmd.Flags().IntVarP(&opts.limit, "limit", "L", queries.LimitDefault, "Maximum number of items to fetch")

	return listCmd
}

func runList(config listConfig) error {
	if config.opts.query != "" {
		features, err := config.detector.ProjectFeatures()
		if err != nil {
			return err
		}
		if !features.ProjectItemQuery {
			return fmt.Errorf("the `--query` flag is not supported on this GitHub host")
		}
	}

	canPrompt := config.io.CanPrompt()
	owner, err := config.client.NewOwner(canPrompt, config.opts.owner)
	if err != nil {
		return err
	}

	// no need to fetch the project if we already have the number
	if config.opts.number == 0 {
		project, err := config.client.NewProject(canPrompt, owner, config.opts.number, false, nil)
		if err != nil {
			return err
		}
		config.opts.number = project.Number
	}

	project, err := config.client.ProjectItems(owner, config.opts.number, config.opts.limit, config.opts.query)
	if err != nil {
		return err
	}

	if config.opts.exporter != nil {
		return config.opts.exporter.Write(config.io, project.DetailedItems())
	}

	// Resolve any requested extra field columns to (header, fieldID) pairs. Name
	// columns are resolved against the fields already returned with the items, so
	// there is no separate field-ID preflight lookup. When extra columns are
	// requested for a project whose field list spans more than one page, fetch the
	// complete field list first so resolution does not miss fields beyond the
	// first page.
	fields := project.Fields.Nodes
	if (len(config.opts.fields) > 0 || len(config.opts.fieldIDs) > 0) && project.Fields.PageInfo.HasNextPage {
		withFields, err := config.client.ProjectFields(owner, config.opts.number, project.Fields.TotalCount)
		if err != nil {
			return err
		}
		fields = withFields.Fields.Nodes
	}

	extraFields, err := resolveFieldColumns(config.opts, fields)
	if err != nil {
		return err
	}

	return printResults(config, project.Items.Nodes, owner.Login, extraFields)
}

// fieldColumn identifies an extra table column to render for each item: Header
// is the column title and FieldID is the field node ID whose value to show.
type fieldColumn struct {
	Header  string
	FieldID string
}

// resolveFieldColumns turns the --field (names) and --field-id (node IDs) flags
// into ordered column descriptors. Names are resolved with ResolveFieldByName so
// unknown or ambiguous names fail with candidate-carrying errors; IDs are
// validated against the project's fields so an unknown ID also fails clearly.
func resolveFieldColumns(opts listOpts, fields []queries.ProjectField) ([]fieldColumn, error) {
	columns := make([]fieldColumn, 0, len(opts.fields)+len(opts.fieldIDs))

	for _, name := range opts.fields {
		field, err := queries.ResolveFieldByName(fields, name)
		if err != nil {
			return nil, err
		}
		columns = append(columns, fieldColumn{Header: field.Name(), FieldID: field.ID()})
	}

	for _, id := range opts.fieldIDs {
		field, err := queries.ResolveFieldByID(fields, id)
		if err != nil {
			return nil, err
		}
		columns = append(columns, fieldColumn{Header: field.Name(), FieldID: field.ID()})
	}

	return columns, nil
}

func printResults(config listConfig, items []queries.ProjectItem, login string, extraFields []fieldColumn) error {
	if len(items) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("Project %d for owner %s has no items", config.opts.number, login))
	}

	headers := []string{"Type", "Title", "Number", "Repository", "ID"}
	for _, f := range extraFields {
		headers = append(headers, f.Header)
	}

	tp := tableprinter.New(config.io, tableprinter.WithHeader(headers...))

	for _, i := range items {
		tp.AddField(i.Type())
		tp.AddField(i.Title())
		if i.Number() == 0 {
			tp.AddField("")
		} else {
			tp.AddField(strconv.Itoa(i.Number()))
		}
		tp.AddField(i.Repo())
		tp.AddField(i.ID(), tableprinter.WithTruncate(nil))
		for _, f := range extraFields {
			tp.AddField(fieldValueForItem(i, f.FieldID))
		}
		tp.EndRow()
	}

	return tp.Render()
}

// fieldValueForItem returns the display value of the field identified by fieldID
// on item i, or an empty string when the item has no value for that field.
func fieldValueForItem(i queries.ProjectItem, fieldID string) string {
	for _, v := range i.FieldValues.Nodes {
		if v.ID() == fieldID {
			return v.DisplayValue()
		}
	}
	return ""
}
