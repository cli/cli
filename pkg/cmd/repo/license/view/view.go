package view

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	License    string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		License:    "",
	}

	cmd := &cobra.Command{
		Use:   "view <license>",
		Short: "View an available repository license template",
		Long: heredoc.Docf(`
		View an available repository license template.
		
		%[1]s<license>%[1]s is a license name or SPDX ID.
		`, "`"),
		Args: cmdutil.ExactArgs(1, "gh repo licese view only takes a single license argument"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.License = args[0]
			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}
	return cmd
}

func viewRun(opts *ViewOptions) error {
	if opts.License == "" {
		return errors.New("no license provided")
	}

	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	hostname, _ := cfg.Authentication().DefaultHost()
	license, err := api.LicenseTemplate(client, hostname, opts.License)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return fmt.Errorf("'%s' is not a valid license template name or SPDX ID. Run `gh repo license list` for options", opts.License)
		}
		return err
	}

	return renderLicense(license, opts)
}

func renderLicense(licenseTemplate api.License, opts *ViewOptions) error {
	cs := opts.IO.ColorScheme()
	var out strings.Builder
	if opts.IO.IsStdoutTTY() {
		out.WriteString(fmt.Sprintf("\n%s\n", cs.Gray(licenseTemplate.Description)))
		out.WriteString(fmt.Sprintf("\n%s\n", cs.Grayf("To implement: %s", licenseTemplate.Implementation)))
		out.WriteString(fmt.Sprintf("\n%s\n\n", cs.Grayf("For more information, see: %s", licenseTemplate.HTMLURL)))
	}
	out.WriteString(licenseTemplate.Body)
	_, err := opts.IO.Out.Write([]byte(out.String()))
	if err != nil {
		return err
	}
	return nil
}
