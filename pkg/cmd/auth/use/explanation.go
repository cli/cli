package use

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

func explainReport(f *cmdutil.Factory, r *identity.Report) error {
	out, cs := f.IOStreams.Out, f.IOStreams.ColorScheme()
	heading := func(title string) { fmt.Fprintf(out, "\n%s\n", cs.Bold(title)) }
	if id := r.Selection.Identity; id != nil {
		fmt.Fprintf(out, "Using identity %s (%s on %s)\n", cs.Cyan(fmt.Sprintf("%q", id.Name)), cs.Cyan(id.Account), id.Host)
	} else {
		fmt.Fprintln(out, cs.Bold("No scoped identity applies"))
	}
	heading("Context")
	fmt.Fprintf(out, "  Directory:  %s\n  Host:       %s\n", r.Context.Directory, r.Context.Host)
	if r.Context.Repository != "" {
		fmt.Fprintf(out, "  Repository: %s\n", r.Context.Repository)
		fmt.Fprintf(out, "  Shared Git directory: %s\n", r.Context.CommonGitDir)
		fmt.Fprintf(out, "  Directory choices are matched at: %s\n", r.Context.ScopeDirectory)
	}
	if r.Context.Owner != "" {
		fmt.Fprintf(out, "  Owner:      %s (explicit target)\n", r.Context.Owner)
	}
	if selected := r.Selection.Selected; selected != nil {
		heading("Why this identity")
		if selected.Repository != "" {
			fmt.Fprintf(out, "  Selected: repository %s (main checkout and all worktrees)\n", cs.Cyan(selected.Repository))
		} else if selected.Owner != "" {
			fmt.Fprintf(out, "  Selected: owner %s\n", cs.Cyan(selected.Owner))
		} else {
			fmt.Fprintf(out, "  Selected: directory %s (includes descendants)\n", cs.Cyan(selected.Directory))
		}
		for _, match := range r.Selection.Superseded {
			b := match.Binding
			scope := "directory " + b.Directory
			if b.Owner != "" {
				scope = "owner " + b.Owner
			}
			fmt.Fprintf(out, "  %s\n", cs.Muted(fmt.Sprintf("Overrides %s -> %s: %s", scope, b.Identity, match.Reason)))
		}
	}
	if len(r.Selection.Descendants) > 0 {
		heading("Descendant overrides (do not affect this directory):")
		for _, b := range r.Selection.Descendants {
			if b.Repository != "" {
				fmt.Fprintf(out, "  repository %s (all worktrees) -> %s\n", b.Repository, cs.Cyan(b.Identity))
			} else {
				fmt.Fprintf(out, "  directory %s -> %s\n", b.Directory, cs.Cyan(b.Identity))
			}
		}
	}
	heading("Authentication")
	for _, row := range []struct {
		label  string
		effect identity.Effect
	}{
		{"gh API", r.Authentication.API}, {"HTTPS Git", r.Authentication.HTTPS}, {"SSH Git", r.Authentication.SSH},
	} {
		e := row.effect
		switch e.Status {
		case "declared":
			value := e.Value
			if row.label == "HTTPS Git" {
				value += " (gh credential helper)"
			}
			if row.label == "SSH Git" {
				value += " (SSH host)"
			}
			fmt.Fprintf(out, "  %-10s %s\n", row.label+":", cs.Cyan(value))
			if row.label == "SSH Git" {
				fmt.Fprintf(out, "             %s\n", e.Detail)
			}
		case "blocked", "overridden":
			detail := e.Detail
			if e.Status == "blocked" && e.Value != "" {
				detail = "Cannot use " + e.Value + ": " + detail
			}
			fmt.Fprintf(out, "  %-10s %s\n", row.label+":", cs.Yellow(detail))
		default:
			fmt.Fprintf(out, "  %-10s %s\n", row.label+":", e.Detail)
		}
		if e.Origin != "" {
			fmt.Fprintf(out, "             %s\n", cs.Muted("Source: "+e.Origin))
		}
	}
	heading("Commit attribution")
	if r.Attribution.Status == "observed" {
		fmt.Fprintln(out, cs.Muted("  Git settings for this repository:"))
		for _, row := range []struct {
			key   string
			value identity.ConfigValue
		}{
			{"user.name", r.Attribution.Name}, {"user.email", r.Attribution.Email},
		} {
			if row.value.Origin == "" {
				fmt.Fprintf(out, "  Git %s: not configured\n", row.key)
			} else {
				fmt.Fprintf(out, "  Git %s: %s\n    %s\n", row.key, cs.Cyan(row.value.Value), cs.Muted("Source: "+row.value.Origin))
			}
		}
	} else {
		if r.Attribution.Name.Value != "" {
			fmt.Fprintln(out, "  You're not in a Git repository. This identity sets:")
			fmt.Fprintf(out, "    user.name:  %s\n    user.email: %s\n", cs.Cyan(r.Attribution.Name.Value), cs.Cyan(r.Attribution.Email.Value))
		} else {
			fmt.Fprintln(out, "  You're not in a Git repository.")
			if r.Selection.Identity == nil {
				fmt.Fprintln(out, "  No identity sets a Git name or email here.")
			} else {
				fmt.Fprintln(out, "  This identity sets no Git name or email; broader Git settings may still apply.")
			}
		}
	}
	for _, name := range r.Attribution.Overrides {
		fmt.Fprintf(out, "  %s\n", cs.Yellow("Active commit override: "+name))
	}
	heading("Configuration sources")
	if len(r.Configuration) == 0 {
		fmt.Fprintln(out, "  Git is not configured to include other files here.")
	}
	sources := []string{}
	grouped := map[string][]identity.Include{}
	for _, edge := range r.Configuration {
		if _, ok := grouped[edge.Source]; !ok {
			sources = append(sources, edge.Source)
		}
		grouped[edge.Source] = append(grouped[edge.Source], edge)
	}
	for _, source := range sources {
		fmt.Fprintf(out, "  %s\n", source)
		seen := map[string]bool{}
		for _, edge := range grouped[source] {
			if seen[edge.Target] {
				continue
			}
			seen[edge.Target] = true
			count := 0
			requiresRepository := true
			for _, other := range grouped[source] {
				if other.Target == edge.Target {
					count++
					if !strings.HasPrefix(other.Condition, "gitdir:") && !strings.HasPrefix(other.Condition, "gitdir/i:") {
						requiresRepository = false
					}
				}
			}
			fmt.Fprintf(out, "    -> %s", edge.Target)
			if !edge.TargetObserved {
				if r.Context.Repository == "" && requiresRepository {
					fmt.Fprint(out, cs.Muted("\n       Not applied: this directory is not a Git repository."))
				} else {
					fmt.Fprint(out, cs.Muted(" (no settings loaded here)"))
				}
			}
			fmt.Fprintln(out)
			if count > 1 {
				fmt.Fprintf(out, "       %s\n", cs.Muted(fmt.Sprintf("%d include directives; see --explain --json configuration for conditions", count)))
			} else if edge.Condition != "" {
				fmt.Fprintf(out, "       %s\n", cs.Muted("if "+edge.Condition))
			}
		}
	}
	if len(r.Warnings) > 0 {
		heading("Warnings")
		for _, warning := range r.Warnings {
			fmt.Fprintf(out, "  %s\n", cs.Yellow(warning))
		}
	}
	return nil
}
