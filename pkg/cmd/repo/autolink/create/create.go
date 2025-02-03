package create

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type createOptions struct {
	BaseRepo       func() (ghrepo.Interface, error)
	Browser        browser.Browser
	AutolinkClient AutolinkCreateClient
	IO             *iostreams.IOStreams
	Exporter       cmdutil.Exporter

	KeyPrefix   string
	URLTemplate string
	Numeric     bool
}

type AutolinkCreateClient interface {
	Create(repo ghrepo.Interface, request AutolinkCreateRequest) (*shared.Autolink, error)
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*createOptions) error) *cobra.Command {
	opts := &createOptions{
		Browser: f.Browser,
		IO:      f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "create <keyPrefix> <urlTemplate>",
		Short: "Create a new autolink reference",
		Long: heredoc.Docf(`
			Create a new autolink reference for a repository.

			The %[1]skeyPrefix%[1]s argument specifies the prefix that will generate a link when it is appended by certain characters.

			The %[1]surlTemplate%[1]s argument specifies the target URL that will be generated when the keyPrefix is found, which
			must contain %[1]s<num>%[1]s variable for the reference number.

			By default, autolinks are alphanumeric with %[1]s--numeric%[1]s flag used to create a numeric autolink.

			The %[1]s<num>%[1]s variable behavior differs depending on whether the autolink is alphanumeric or numeric:

			- alphanumeric: matches %[1]sA-Z%[1]s (case insensitive), %[1]s0-9%[1]s, and %[1]s-%[1]s
			- numeric: matches %[1]s0-9%[1]s

			If the template contains multiple instances of %[1]s<num>%[1]s, only the first will be replaced.
		`, "`"),
		Example: heredoc.Doc(`
			# Create an alphanumeric autolink to example.com for the key prefix "TICKET-".
			# Generates https://example.com/TICKET?query=123abc from "TICKET-123abc".
			$ gh repo autolink create TICKET- "https://example.com/TICKET?query=<num>"

			# Create a numeric autolink to example.com for the key prefix "STORY-".
			# Generates https://example.com/STORY?id=123 from "STORY-123".
			$ gh repo autolink create STORY- "https://example.com/STORY?id=<num>" --numeric
		`),
		Args:    cmdutil.ExactArgs(2, "Cannot create autolink: keyPrefix and urlTemplate arguments are both required"),
		Aliases: []string{"new"},
		RunE: func(c *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts.AutolinkClient = &AutolinkCreator{HTTPClient: httpClient}
			opts.KeyPrefix = args[0]
			opts.URLTemplate = args[1]

			if runF != nil {
				return runF(opts)
			}

			return createRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Numeric, "numeric", "n", false, "Mark autolink as numeric")

	return cmd
}

func createRun(opts *createOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	request := AutolinkCreateRequest{
		KeyPrefix:      opts.KeyPrefix,
		URLTemplate:    opts.URLTemplate,
		IsAlphanumeric: !opts.Numeric,
	}

	autolink, err := opts.AutolinkClient.Create(repo, request)
	if err != nil {
		return fmt.Errorf("error creating autolink: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out,
		"%s Created repository autolink %s on %s\n",
		cs.SuccessIconWithColor(cs.Green),
		cs.Cyanf("%d", autolink.ID),
		ghrepo.FullName(repo))

	return nil
}
