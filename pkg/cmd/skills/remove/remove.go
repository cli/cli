package remove

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/skills/installer"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RemoveOptions struct {
	IO        *iostreams.IOStreams
	GitClient interface{} // Not heavily used here, but keeping signature standard
	SkillName string

	Agent string
	Scope string
	Dir   string
}

func NewCmdRemove(f *cmdutil.Factory, runF func(*RemoveOptions) error) *cobra.Command {
	opts := &RemoveOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:     "remove <skill>",
		Short:   "Remove an installed skill",
		Aliases: []string{"uninstall"},
		Long: heredoc.Docf(`
			Remove an installed agent skill from your local environment.

			By default, this command searches across all agent host directories in both
			project and user scopes and removes the skill wherever it is found.

			Use %[1]s--agent%[1]s to restrict removal to a specific host, %[1]s--scope%[1]s to restrict
			to project or user scope, or %[1]s--dir%[1]s to remove from a custom directory.
		`, "`"),
		Example: heredoc.Doc(`
			# Remove a skill from all scopes and agents
			$ gh skill remove documentation-writer

			# Remove a skill only for GitHub Copilot
			$ gh skill remove documentation-writer --agent github-copilot

			# Remove a user-scope skill
			$ gh skill remove documentation-writer --scope user
		`),
		Args: cmdutil.ExactArgs(1, "must specify a skill to remove"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.SkillName = args[0]
			opts.GitClient = f.GitClient

			if err := cmdutil.MutuallyExclusive("--dir and --agent cannot be used together", opts.Dir != "", opts.Agent != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("--dir and --scope cannot be used together", opts.Dir != "", cmd.Flags().Changed("scope")); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}
			return removeRun(opts)
		},
	}

	cmdutil.StringEnumFlag(cmd, &opts.Agent, "agent", "", "", registry.AgentIDs(), "Filter by target agent")
	cmdutil.StringEnumFlag(cmd, &opts.Scope, "scope", "", "", []string{string(registry.ScopeProject), string(registry.ScopeUser)}, "Filter by installation scope")
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "Remove from a custom directory")

	return cmd
}

func removeRun(opts *RemoveOptions) error {
	dirs, err := resolveTargetDirs(opts)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	removedCount := 0

	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			continue // dir doesn't exist
		}

		// Try direct match (flat layout)
		target := filepath.Join(dir, opts.SkillName)
		if stat, err := os.Stat(target); err == nil && stat.IsDir() {
			if err := os.RemoveAll(target); err == nil {
				removedCount++
				if opts.IO.IsStdoutTTY() {
					fmt.Fprintf(opts.IO.Out, "%s Removed %s from %s\n", cs.SuccessIcon(), opts.SkillName, dir)
				}
			}
			continue
		}

		// Try namespaced layout (e.g. if opts.SkillName is 'documentation-writer', it might be at 'owner/documentation-writer')
		if !strings.Contains(opts.SkillName, "/") {
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, e := range entries {
					if e.IsDir() {
						nestedTarget := filepath.Join(dir, e.Name(), opts.SkillName)
						if stat, err := os.Stat(nestedTarget); err == nil && stat.IsDir() {
							if err := os.RemoveAll(nestedTarget); err == nil {
								removedCount++
								if opts.IO.IsStdoutTTY() {
									fmt.Fprintf(opts.IO.Out, "%s Removed %s from %s\n", cs.SuccessIcon(), opts.SkillName, filepath.Join(dir, e.Name()))
								}
								// Remove empty parent directory if needed
								_ = os.Remove(filepath.Join(dir, e.Name()))
							}
						}
					}
				}
			}
		}
	}

	if removedCount == 0 {
		return fmt.Errorf("skill %q not found", opts.SkillName)
	}

	return nil
}

func resolveTargetDirs(opts *RemoveOptions) ([]string, error) {
	if opts.Dir != "" {
		absDir, err := filepath.Abs(opts.Dir)
		if err != nil {
			return nil, fmt.Errorf("could not resolve path: %w", err)
		}
		return []string{absDir}, nil
	}

	var gitRoot string
	if client, ok := opts.GitClient.(*git.Client); ok {
		gitRoot = installer.ResolveGitRoot(client)
	}
	homeDir := installer.ResolveHomeDir()

	var hosts []registry.AgentHost
	if opts.Agent != "" {
		for _, h := range registry.Agents {
			if h.ID == opts.Agent {
				hosts = append(hosts, h)
				break
			}
		}
	} else {
		hosts = registry.Agents
	}

	var scopes []registry.Scope
	if opts.Scope != "" {
		scopes = append(scopes, registry.Scope(opts.Scope))
	} else {
		scopes = []registry.Scope{registry.ScopeProject, registry.ScopeUser}
	}

	var dirs []string
	seen := map[string]bool{}

	for _, h := range hosts {
		for _, s := range scopes {
			if dir, err := h.InstallDir(s, gitRoot, homeDir); err == nil {
				if !seen[dir] {
					dirs = append(dirs, dir)
					seen[dir] = true
				}
			}
		}
	}

	return dirs, nil
}
