package add_license

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const ChooseLicenseUrl = "https://choosealicense.com"

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
}

type AddLicenseOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	Prompter   iprompter

	Name            string
	List            bool
	LicenseTemplate string
	Interactive     bool
	Info            bool
	IO              *iostreams.IOStreams
}

func NewCmdAddLicense(f *cmdutil.Factory, runF func(*AddLicenseOptions) error) *cobra.Command {
	opts := &AddLicenseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Prompter:   f.Prompter,
		Browser:    f.Browser,
	}
	cmd := &cobra.Command{
		Use:   "add-license [<repository>]",
		Short: "Add a license to an existing repository",
		Long: heredoc.Doc(`Add a license to an existing GitHub repository.

With no argument, archives the current repository.`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.List {
				if len(args) > 0 || cmd.Flags().NFlag() > 1 {
					return cmdutil.FlagErrorf("expected no arguments or flags with --list")
				}
			} else if opts.Info {
				if len(args) > 0 || cmd.Flags().NFlag() > 1 {
					return cmdutil.FlagErrorf("expected no arguments or flags with --info")
				}
			} else {
				if len(args) > 0 {
					opts.Name = args[0]
				}

				if len(args) == 0 && cmd.Flags().NFlag() == 0 {
					if !opts.IO.CanPrompt() {
						return cmdutil.FlagErrorf("at least one argument required in non-interactive mode")
					}
					opts.Interactive = true
				} else {
					opts.Interactive = false
				}
			}

			if runF != nil {
				return runF(opts)
			}

			return addLicenseRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.List, "list", "l", false, "List license templates")
	cmd.Flags().BoolVarP(&opts.Info, "info", "i", false, "View license information in the browser")

	return cmd
}

func addLicenseRun(opts *AddLicenseOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	if opts.List {
		out := opts.IO.Out

		licenses, err := listLicenseTemplates(httpClient, host)
		if err != nil {
			return err
		}

		maxNameLength := 0
		maxKeyLength := 0
		for _, license := range licenses {
			nameLength := len(license.Name)
			keyLength := len(license.Key)

			if nameLength > maxNameLength {
				maxNameLength = nameLength
			}

			if keyLength > maxKeyLength {
				maxKeyLength = keyLength
			}
		}

		fmt.Fprintf(out, "Available licenses: \n")
		fmt.Fprintf(out, "%-*s | %s\n", maxNameLength, "Name", "Key")
		fmt.Fprintf(out, "%s|%s\n", strings.Repeat("-", maxNameLength+1), strings.Repeat("-", maxKeyLength))
		for _, license := range licenses {
			fmt.Fprintf(out, "%-*s | %s\n", maxNameLength, license.Name, license.Key)
		}

		return nil
	}

	if opts.Interactive {
		opts.LicenseTemplate, err = interactiveLicense(httpClient, host, opts.Prompter)
		if err != nil {
			return err
		}
	}

	if opts.Info || opts.LicenseTemplate == "" {
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(ChooseLicenseUrl))
		}
		return opts.Browser.Browse(ChooseLicenseUrl)
	}

	return nil
}
