package view

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	License    string
	Web        bool
	Browser    browser.Browser
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view <license>",
		Short: "View an available repository license template",
		Long: heredoc.Docf(`
			View an available repository license template.
			
			%[1]s<license>%[1]s can be a case-insensitive license name or SPDX ID.

			Run %[1]sgh repo license list%[1]s to see available commonly used licenses. For even more licenses, visit <https://choosealicense.com/appendix>.
		`, "`"),
		Example: heredoc.Doc(`
			# View the MIT license
			gh repo license view MIT

			# View the GNU AGPL-3.0 license
			gh repo license view AGPL-3.0

			# View the Apache-2.0 license
			gh repo license view Apache-2.0

			# Create a LICENSE.md with the MIT license
			gh repo license view MIT > LICENSE.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.License = args[0]
			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open https://choosealicense.com/ in the browser")

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

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	hostname, _ := cfg.Authentication().DefaultHost()
	license, err := api.RepoLicense(client, hostname, opts.License)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return fmt.Errorf("'%s' is not a valid license template name or SPDX ID.\n\nRun `gh repo license list` to see available commonly used licenses. For even more licenses, visit %s", opts.License, text.DisplayURL("https://choosealicense.com/appendix"))
		}
		return err
	}

	if opts.Web {
		url := fmt.Sprintf("https://choosealicense.com/licenses/%s", license.Key)
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
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
