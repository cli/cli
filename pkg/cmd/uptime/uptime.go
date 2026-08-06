package uptime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// statusURL is the public GitHub Status (Statuspage) summary endpoint. It is
// deliberately NOT api.github.com and requires no authentication, so this
// command never sends the user's GitHub token to a third-party host.
const statusURL = "https://www.githubstatus.com/api/v2/summary.json"

// componentOrder pins the display order to the grouping used on
// githubstatus.com; anything unlisted sorts alphabetically after these.
var componentOrder = map[string]int{
	"Git Operations":     0,
	"API Requests":       1,
	"Webhooks":           2,
	"Issues":             3,
	"Pull Requests":      4,
	"Actions":            5,
	"Packages":           6,
	"Pages":              7,
	"Codespaces":         8,
	"Copilot":            9,
	"Copilot Extensions": 10,
}

type summary struct {
	Status struct {
		Indicator   string `json:"indicator"`   // none | minor | major | critical
		Description string `json:"description"` // e.g. "All Systems Operational"
	} `json:"status"`
	Components []component `json:"components"`
}

type component struct {
	Name   string `json:"name"`
	Status string `json:"status"` // operational | degraded_performance | partial_outage | major_outage
	Group  bool   `json:"group"`
}

// UptimeOptions holds the resolved dependencies + flags for `gh uptime`.
type UptimeOptions struct {
	HTTPClient func() *http.Client
	IO         *iostreams.IOStreams
	Exporter   cmdutil.Exporter

	Component string
	Watch     bool
	Interval  time.Duration
	ExitCode  bool
}

func NewCmdUptime(f *cmdutil.Factory, runF func(*UptimeOptions) error) *cobra.Command {
	opts := &UptimeOptions{
		IO: f.IOStreams,
		// A plain client — the factory's HttpClient injects the gh auth token,
		// which must never be sent to githubstatus.com.
		HTTPClient: func() *http.Client {
			return &http.Client{Timeout: 15 * time.Second}
		},
	}

	cmd := &cobra.Command{
		Use:   "uptime",
		Short: "Show the operational status of GitHub services",
		Long: heredoc.Docf(`
			Report the current operational status of GitHub's services, as published
			at <https://www.githubstatus.com>.

			This queries the public GitHub Status page (not api.github.com) and sends
			no authentication, so it works even when your token is unset or when the
			API itself is degraded.

			With %[1]s--watch%[1]s the command polls until GitHub (or a single named
			%[1]s--component%[1]s) returns to operational — useful for holding a script
			until an incident clears. Combined with %[1]s--exit-code%[1]s it composes
			directly into automation:

			    # wait out an Actions outage, then kick off a workflow
			    gh uptime --component Actions --watch && gh workflow run ci.yml
		`, "`"),
		Args: cobra.NoArgs,
		Example: heredoc.Doc(`
			# current status of every GitHub service
			$ gh uptime

			# just Actions, machine-readable, for a polling loop
			$ gh uptime --component Actions --json component,status,indicator

			# fail (non-zero exit) if anything is degraded — gate a deploy
			$ gh uptime --exit-code || echo "GitHub degraded; holding"

			# block until Actions recovers, checking every 30s
			$ gh uptime --component Actions --watch --interval 30s
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Interval < 5*time.Second {
				return cmdutil.FlagErrorf("--interval must be at least 5s to respect the status page")
			}
			if runF != nil {
				return runF(opts)
			}
			return uptimeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Component, "component", "c", "", "Report a single component by name (e.g. \"Actions\")")
	cmd.Flags().BoolVarP(&opts.Watch, "watch", "w", false, "Poll until the target is operational")
	cmd.Flags().DurationVarP(&opts.Interval, "interval", "i", 30*time.Second, "Polling interval for --watch")
	cmd.Flags().BoolVar(&opts.ExitCode, "exit-code", false, "Exit non-zero when the target is not operational")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, uptimeFields)

	// No GitHub auth needed to read a public status page.
	cmdutil.DisableAuthCheck(cmd)

	return cmd
}

func uptimeRun(opts *UptimeOptions) error {
	client := opts.HTTPClient()

	for {
		s, err := fetchSummary(client)
		if err != nil {
			return err
		}

		operational := targetOperational(s, opts.Component)

		// In watch mode, keep polling silently until operational, then render once.
		if opts.Watch && !operational {
			time.Sleep(opts.Interval)
			continue
		}

		if opts.Exporter != nil {
			if err := opts.Exporter.Write(opts.IO, exportShape(s, opts.Component)); err != nil {
				return err
			}
		} else if err := render(opts.IO, s, opts.Component); err != nil {
			return err
		}

		if opts.ExitCode && !operational {
			return cmdutil.SilentError
		}
		return nil
	}
}

func fetchSummary(client *http.Client) (*summary, error) {
	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the GitHub status page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub status page returned %s", resp.Status)
	}
	var s summary
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("could not parse the GitHub status page response: %w", err)
	}
	return &s, nil
}

// targetOperational reports whether the whole platform (component == "") or a
// single named component is currently operational.
func targetOperational(s *summary, name string) bool {
	if name == "" {
		return s.Status.Indicator == "none"
	}
	for _, c := range s.Components {
		if c.Group {
			continue
		}
		if strings.EqualFold(c.Name, name) {
			return c.Status == "operational"
		}
	}
	return false
}

// leafComponents returns the real, non-group service components in a stable
// display order. The status page carries a couple of non-service rows (e.g. a
// "Visit www.githubstatus.com for more information" link) as components; those
// are dropped so callers only ever see actual GitHub services.
func leafComponents(s *summary) []component {
	out := make([]component, 0, len(s.Components))
	for _, c := range s.Components {
		if c.Group {
			continue
		}
		if strings.HasPrefix(c.Name, "Visit ") || strings.Contains(c.Name, "githubstatus.com") {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, iok := componentOrder[out[i].Name]
		oj, jok := componentOrder[out[j].Name]
		if iok && jok {
			return oi < oj
		}
		if iok != jok {
			return iok
		}
		return out[i].Name < out[j].Name
	})
	return out
}
