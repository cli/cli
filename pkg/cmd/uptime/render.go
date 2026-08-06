package uptime

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// uptimeFields are the columns available to --json.
var uptimeFields = []string{
	"indicator",
	"description",
	"component",
	"status",
	"operational",
	"components",
}

// exportedComponent is the per-component JSON shape.
type exportedComponent struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Operational bool   `json:"operational"`
}

// exportShape builds the object returned by --json/--jq. When a single
// component is requested the top-level status/operational reflect just that
// component, so `gh uptime -c Actions --json operational` is a clean boolean.
func exportShape(s *summary, name string) map[string]interface{} {
	comps := leafComponents(s)
	exported := make([]exportedComponent, 0, len(comps))
	for _, c := range comps {
		exported = append(exported, exportedComponent{
			Name:        c.Name,
			Status:      c.Status,
			Operational: c.Status == "operational",
		})
	}

	out := map[string]interface{}{
		"indicator":   s.Status.Indicator,
		"description": s.Status.Description,
		"components":  exported,
		"operational": targetOperational(s, name),
	}

	if name != "" {
		out["component"] = name
		status := "unknown"
		for _, c := range comps {
			if strings.EqualFold(c.Name, name) {
				status = c.Status
				break
			}
		}
		out["status"] = status
	} else {
		out["component"] = nil
		out["status"] = s.Status.Indicator
	}

	return out
}

// render prints the human-readable report.
func render(io *iostreams.IOStreams, s *summary, name string) error {
	cs := io.ColorScheme()
	out := io.Out

	// Single-component view.
	if name != "" {
		for _, c := range leafComponents(s) {
			if strings.EqualFold(c.Name, name) {
				fmt.Fprintf(out, "%s %s %s\n", statusIcon(cs, c.Status), c.Name, statusText(cs, c.Status))
				return nil
			}
		}
		return fmt.Errorf("no such GitHub component %q; run `gh uptime` to list components", name)
	}

	// Overall banner.
	fmt.Fprintf(out, "%s %s\n", overallIcon(cs, s.Status.Indicator), cs.Bold(s.Status.Description))

	if !io.IsStdoutTTY() {
		// Non-TTY: keep the per-line output stable and unstyled-ish for scripts
		// that grep it (colour is auto-disabled by the ColorScheme already).
	}

	fmt.Fprintln(out)
	for _, c := range leafComponents(s) {
		fmt.Fprintf(out, "  %s %-22s %s\n", statusIcon(cs, c.Status), c.Name, statusText(cs, c.Status))
	}
	return nil
}

func overallIcon(cs *iostreams.ColorScheme, indicator string) string {
	switch indicator {
	case "none":
		return cs.SuccessIcon()
	case "minor":
		return cs.WarningIcon()
	default: // major, critical
		return cs.FailureIcon()
	}
}

func statusIcon(cs *iostreams.ColorScheme, status string) string {
	switch status {
	case "operational":
		return cs.SuccessIcon()
	case "degraded_performance", "partial_outage":
		return cs.WarningIcon()
	default: // major_outage, unknown
		return cs.FailureIcon()
	}
}

// statusText renders the human label (e.g. "major_outage" -> "major outage")
// in a colour matching severity.
func statusText(cs *iostreams.ColorScheme, status string) string {
	label := strings.ReplaceAll(status, "_", " ")
	switch status {
	case "operational":
		return cs.Green(label)
	case "degraded_performance", "partial_outage":
		return cs.Yellow(label)
	default:
		return cs.Red(label)
	}
}
