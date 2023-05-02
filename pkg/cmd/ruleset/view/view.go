package view

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	ID           string
	WebMode      bool
	Organization string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "view [<ruleset-id>]",
		Short: "View information about a ruleset",
		Long: heredoc.Doc(`
			View information about a GitHub ruleset.

			If no ID is provided, an interactive prompt will be used to choose
			the ruleset to view.
		`),
		Example: heredoc.Doc(`
			# View a ruleset in the current repository
			$ gh ruleset view 43

			# View a ruleset in a different repository
			$ gh ruleset view 23 --repo owner/repo

			# View an organization-level ruleset
			$ gh ruleset view 23 --org my-org
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				// a string is actually needed later on, so verify that it's numeric
				// but use the string anyway
				_, err := strconv.Atoi(args[0])
				if err != nil {
					return cmdutil.FlagErrorf("invalid value for ruleset ID: %v is not an integer", args[0])
				}
				opts.ID = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the ruleset in the browser")
	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "Organization name if the provided ID is an organization-level ruleset")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repoI, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname, _ := cfg.DefaultHost()

	var rs *shared.RulesetREST
	if opts.Organization != "" {
		rs, err = viewOrgRuleset(httpClient, opts.Organization, opts.ID, hostname)
	} else {
		rs, err = viewRepoRuleset(httpClient, repoI, opts.ID)
	}

	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	w := opts.IO.Out

	if opts.WebMode {
		if rs != nil {
			var rulesetURL string
			if opts.Organization != "" {
				rulesetURL = fmt.Sprintf("%sorganizations/%s/settings/rules/%s", ghinstance.HostPrefix(hostname), opts.Organization, opts.ID)
			} else {
				rulesetURL = ghrepo.GenerateRepoURL(repoI, "settings/rules/%s", opts.ID)
			}

			if opts.IO.IsStdoutTTY() {
				fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rulesetURL))
			}

			return opts.Browser.Browse(rulesetURL)
		} else {
			fmt.Fprintf(w, "ruleset not found\n")
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold(rs.Name))
	fmt.Fprintf(w, "ID: %d\n", rs.Id)

	switch rs.Enforcement {
	case "disabled":
		fmt.Fprintf(w, "%s\n", cs.Red("Disabled"))
	case "evaluate":
		fmt.Fprintf(w, "%s\n", cs.Yellow("Evaluate Mode (not enforced)"))
	case "active":
		fmt.Fprintf(w, "%s\n", cs.Green("Active"))
	default:
		fmt.Fprintf(w, "Enforcement: %s\n", rs.Enforcement)
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Bypassing"))
	fmt.Fprintf(w, "Mode: %s\n", rs.BypassMode)
	if len(rs.BypassActors) == 0 {
		fmt.Fprintf(w, "No actors configured for bypass\n")
	} else {
		types := make(map[string]int)
		for _, t := range rs.BypassActors {
			val, exists := types[t.ActorType]
			if exists {
				types[t.ActorType] = val + 1
			} else {
				types[t.ActorType] = 1
			}
		}

		fmt.Fprintf(w, "Actor types allowed to bypass:\n")
		for name, count := range types {
			fmt.Fprintf(w, "- %s: %d configured\n", name, count)
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Conditions"))
	if len(rs.Conditions) == 0 {
		fmt.Fprintf(w, "No conditions configured\n")
	} else {
		// sort keys for consistent responses, can't make a separate function due to
		// mismatched types
		keys := make([]string, 0, len(rs.Conditions))
		for key := range rs.Conditions {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, name := range keys {
			condition := rs.Conditions[name]
			fmt.Fprintf(w, "- %s: ", name)

			// sort these keys too for consistency
			subkeys := make([]string, 0, len(condition))
			for subkey := range condition {
				subkeys = append(subkeys, subkey)
			}
			sort.Strings(subkeys)

			for _, n := range subkeys {
				rawVal := condition[n]

				k := reflect.TypeOf(rawVal).Kind()
				if rawVal == nil ||
					((k == reflect.Slice || k == reflect.Map) && len(rawVal.([]interface{})) == 0) {
					continue
				}

				printVal := fmt.Sprint(rawVal)

				// fmt.Fprintf(w, "n: %s, type: %s\n", n, reflect.TypeOf(rawVal).String())

				// switch val := rawVal.(type) {
				// case []interface{}:
				// 	// currently only string arrays are returned by the API at this level
				// 	printVal = fmt.Sprint(val)
				// default:
				// 	printVal = fmt.Sprint(val)
				// }

				fmt.Fprintf(w, "[%s: %s] ", n, printVal)
			}

			fmt.Fprint(w, "\n")
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Rules"))
	fmt.Fprintf(w, "%d configured\n", reflect.ValueOf(rs.Rules).Len())

	return nil
}
