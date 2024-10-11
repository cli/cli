package view

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	Prompter   prompter.Prompter

	ID              string
	WebMode         bool
	IncludeParents  bool
	InteractiveMode bool
	Organization    string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "view [<ruleset-id>]",
		Short: "View information about a ruleset",
		Long: heredoc.Docf(`
			View information about a GitHub ruleset.

			If no ID is provided, an interactive prompt will be used to choose
			the ruleset to view.
			
			Use the %[1]s--parents%[1]s flag to control whether rulesets configured at higher
			levels that also apply to the provided repository or organization should
			be returned. The default is %[1]strue%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# Interactively choose a ruleset to view from all rulesets that apply to the current repository
			$ gh ruleset view

			# Interactively choose a ruleset to view from only rulesets configured in the current repository
			$ gh ruleset view --no-parents

			# View a ruleset configured in the current repository or any of its parents
			$ gh ruleset view 43

			# View a ruleset configured in a different repository or any of its parents
			$ gh ruleset view 23 --repo owner/repo

			# View an organization-level ruleset
			$ gh ruleset view 23 --org my-org
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && opts.Organization != "" {
				return cmdutil.FlagErrorf("only one of --repo and --org may be specified")
			}

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
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("a ruleset ID must be provided when not running interactively")
			} else {
				opts.InteractiveMode = true
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the ruleset in the browser")
	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "Organization name if the provided ID is an organization-level ruleset")
	cmd.Flags().BoolVarP(&opts.IncludeParents, "parents", "p", true, "Whether to include rulesets configured at higher levels that also apply")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var repoI ghrepo.Interface

	// only one of the repo or org context is necessary
	if opts.Organization == "" {
		var repoErr error
		repoI, repoErr = opts.BaseRepo()
		if repoErr != nil {
			return repoErr
		}
	}

	hostname, _ := ghauth.DefaultHost()
	cs := opts.IO.ColorScheme()

	if opts.InteractiveMode {
		var rsList *shared.RulesetList
		limit := 30
		if opts.Organization != "" {
			rsList, err = shared.ListOrgRulesets(httpClient, opts.Organization, limit, hostname, opts.IncludeParents)
		} else {
			rsList, err = shared.ListRepoRulesets(httpClient, repoI, limit, opts.IncludeParents)
		}

		if err != nil {
			return err
		}

		if rsList.TotalCount == 0 {
			return shared.NoRulesetsFoundError(opts.Organization, repoI, opts.IncludeParents)
		}

		rs, err := selectRulesetID(rsList, opts.Prompter, cs)
		if err != nil {
			return err
		}

		if rs != nil {
			opts.ID = strconv.Itoa(rs.DatabaseId)

			// can't get a ruleset lower in the chain than what was queried, so no need to handle repos here
			if rs.Source.TypeName == "Organization" {
				opts.Organization = rs.Source.Owner
			}
		}
	}

	var rs *shared.RulesetREST
	if opts.Organization != "" {
		rs, err = viewOrgRuleset(httpClient, opts.Organization, opts.ID, hostname)
	} else {
		rs, err = viewRepoRuleset(httpClient, repoI, opts.ID)
	}

	if err != nil {
		return err
	}

	w := opts.IO.Out

	if opts.WebMode {
		if rs != nil {
			if opts.IO.IsStdoutTTY() {
				fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rs.Links.Html.Href))
			}

			return opts.Browser.Browse(rs.Links.Html.Href)
		} else {
			fmt.Fprintf(w, "ruleset not found\n")
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold(rs.Name))
	fmt.Fprintf(w, "ID: %s\n", cs.Cyan(strconv.Itoa(rs.Id)))
	fmt.Fprintf(w, "Source: %s (%s)\n", rs.Source, rs.SourceType)

	fmt.Fprint(w, "Enforcement: ")
	switch rs.Enforcement {
	case "disabled":
		fmt.Fprintf(w, "%s\n", cs.Red("Disabled"))
	case "evaluate":
		fmt.Fprintf(w, "%s\n", cs.Yellow("Evaluate Mode (not enforced)"))
	case "active":
		fmt.Fprintf(w, "%s\n", cs.Green("Active"))
	default:
		fmt.Fprintf(w, "%s\n", rs.Enforcement)
	}

	if rs.CurrentUserCanBypass != "" {
		fmt.Fprintf(w, "You can bypass: %s\n", strings.ReplaceAll(rs.CurrentUserCanBypass, "_", " "))
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Bypass List"))
	if len(rs.BypassActors) == 0 {
		fmt.Fprintf(w, "This ruleset cannot be bypassed\n")
	} else {
		sort.Slice(rs.BypassActors, func(i, j int) bool {
			return rs.BypassActors[i].ActorId < rs.BypassActors[j].ActorId
		})

		for _, t := range rs.BypassActors {
			fmt.Fprintf(w, "- %s (ID: %d), mode: %s\n", t.ActorType, t.ActorId, t.BypassMode)
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Conditions"))
	if len(rs.Conditions) == 0 {
		fmt.Fprintf(w, "No conditions configured\n")
	} else {
		// sort keys for consistent responses
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
				fmt.Fprintf(w, "[%s: %v] ", n, condition[n])
			}

			fmt.Fprint(w, "\n")
		}
	}

	fmt.Fprintf(w, "\n%s\n", cs.Bold("Rules"))
	if len(rs.Rules) == 0 {
		fmt.Fprintf(w, "No rules configured\n")
	} else {
		fmt.Fprint(w, shared.ParseRulesForDisplay(rs.Rules))
	}

	return nil
}

func selectRulesetID(rsList *shared.RulesetList, p prompter.Prompter, cs *iostreams.ColorScheme) (*shared.RulesetGraphQL, error) {
	rulesets := make([]string, len(rsList.Rulesets))
	for i, rs := range rsList.Rulesets {
		s := fmt.Sprintf(
			"%s: %s | %s | contains %s | configured in %s",
			cs.Cyan(strconv.Itoa(rs.DatabaseId)),
			rs.Name,
			strings.ToLower(rs.Enforcement),
			text.Pluralize(rs.Rules.TotalCount, "rule"),
			shared.RulesetSource(rs),
		)
		rulesets[i] = s
	}

	r, err := p.Select("Which ruleset would you like to view?", rulesets[0], rulesets)
	if err != nil {
		return nil, err
	}

	return &rsList.Rulesets[r], nil
}
