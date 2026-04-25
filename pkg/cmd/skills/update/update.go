package update

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/installer"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/internal/skills/source"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// UpdateOptions holds all dependencies and user-provided flags for the update command.
type UpdateOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	Prompter   prompter.Prompter
	GitClient  *git.Client

	Skills []string
	All    bool
	Force  bool
	DryRun bool
	Unpin  bool
	Dir    string
}

// installedSkill represents a locally installed skill parsed from its SKILL.md frontmatter.
type installedSkill struct {
	name        string
	repoHost    string
	owner       string
	repo        string
	treeSHA     string // tree SHA at install time
	pinned      string // explicit pin value (empty = unpinned)
	sourcePath  string // original path in source repo (e.g. "skills/author/name")
	dir         string // local directory path
	host        *registry.AgentHost
	scope       registry.Scope
	metadataErr error
}

// pendingUpdate describes a single skill that has an available update.
type pendingUpdate struct {
	local    installedSkill
	newSHA   string // new tree SHA from remote
	resolved *discovery.ResolvedRef
	skill    discovery.Skill
}

// NewCmdUpdate creates the "skills update" command.
func NewCmdUpdate(f *cmdutil.Factory, runF func(*UpdateOptions) error) *cobra.Command {
	opts := &UpdateOptions{
		IO:         f.IOStreams,
		Prompter:   f.Prompter,
		Config:     f.Config,
		GitClient:  f.GitClient,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "update [<skill>...] [flags]",
		Short: "Update installed skills to their latest versions (preview)",
		Long: heredoc.Docf(`
			Checks installed skills for available updates by comparing the local
			tree SHA (from %[1]sSKILL.md%[1]s frontmatter) against the remote repository.

			Scans all known agent host directories (Copilot, Claude, Cursor, Codex,
			Gemini, Antigravity) in both project and user scope automatically.

			Without arguments, checks all installed skills. With skill names,
			checks only those specific skills.

			Pinned skills (installed with %[1]s--pin%[1]s) are skipped with a notice.
			Use %[1]s--unpin%[1]s to clear the pinned version and include those skills
			in the update.

			Skills without GitHub metadata (e.g. installed manually or by another
			tool) are prompted for their source repository in interactive mode.
			The update re-downloads the skill with metadata injected, so future
			updates work automatically.

			With %[1]s--force%[1]s, re-downloads skills even when the remote version matches
			the local tree SHA. This overwrites locally modified skill files with
			their original content, but does not remove extra files added locally.

			In interactive mode, shows which skills have updates and asks for
			confirmation before proceeding. With %[1]s--all%[1]s, updates without prompting.
			With %[1]s--dry-run%[1]s, reports available updates without modifying any files.
		`, "`"),
		Example: heredoc.Doc(`
			# Check and update all skills interactively
			$ gh skill update

			# Update specific skills
			$ gh skill update mcp-cli git-commit

			# Update all without prompting
			$ gh skill update --all

			# Re-download all skills (restore locally modified files)
			$ gh skill update --force --all

			# Check for updates without applying (read-only)
			$ gh skill update --dry-run

			# Unpin skills and update them to latest
			$ gh skill update --unpin
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Skills = args
			if runF != nil {
				return runF(opts)
			}
			return updateRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.All, "all", false, "Update all skills without prompting")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Re-download even if already up to date")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Report available updates without modifying files")
	cmd.Flags().BoolVar(&opts.Unpin, "unpin", false, "Clear pinned version and include pinned skills in update")
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "Scan a custom directory for installed skills")

	return cmd
}

func updateRun(opts *UpdateOptions) error {
	cs := opts.IO.ColorScheme()
	canPrompt := opts.IO.CanPrompt()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	gitRoot := installer.ResolveGitRoot(opts.GitClient)
	homeDir := installer.ResolveHomeDir()

	// Scan for installed skills
	var installed []installedSkill
	if opts.Dir != "" {
		skills, scanErr := scanInstalledSkills(opts.Dir, nil, "")
		if scanErr != nil {
			return fmt.Errorf("could not scan directory: %w", scanErr)
		}
		installed = skills
	} else {
		installed = scanAllAgents(gitRoot, homeDir)
	}

	if len(installed) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No installed skills found.\n")
		return nil
	}

	// Filter to requested skills if specified
	if len(opts.Skills) > 0 {
		requested := make(map[string]bool, len(opts.Skills))
		for _, name := range opts.Skills {
			requested[name] = true
		}
		var filtered []installedSkill
		for _, s := range installed {
			if requested[s.name] {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("none of the specified skills are installed")
		}
		installed = filtered
	}

	// Skip skills with invalid metadata rather than aborting the entire
	// update run. One corrupt skill should not prevent updating others.
	{
		var valid []installedSkill
		for _, s := range installed {
			if s.metadataErr != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s Skipping %s: invalid repository metadata: %s\n", cs.WarningIcon(), s.name, s.metadataErr)
				continue
			}
			valid = append(valid, s)
		}
		installed = valid
	}

	if len(installed) == 0 {
		fmt.Fprintf(opts.IO.ErrOut, "No updatable skills found.\n")
		return nil
	}

	// Prompt for metadata on skills missing it (before starting progress indicator)
	var noMeta []string
	// Track skills where the user provided a source repo interactively.
	// Keyed by directory path to avoid collisions when the same skill name
	// is installed across multiple hosts or scopes.
	type promptedEntry struct {
		name   string
		source string // "owner/repo"
	}
	prompted := make(map[string]promptedEntry) // dir > entry
	for i := range installed {
		s := &installed[i]
		if s.owner != "" && s.repo != "" {
			continue
		}
		if !canPrompt {
			noMeta = append(noMeta, s.name)
			continue
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s %s has no GitHub metadata\n", cs.WarningIcon(), s.name)
		owner, repo, reason, ok, promptErr := promptForSkillOrigin(opts.Prompter, s.name)
		if promptErr != nil {
			return promptErr
		}
		if !ok {
			if reason != "" {
				fmt.Fprintf(opts.IO.ErrOut, "  %s %s\n", cs.WarningIcon(), reason)
			}
			fmt.Fprintf(opts.IO.ErrOut, "  Skipping %s\n", s.name)
			continue
		}
		s.owner = owner
		s.repo = repo
		s.repoHost = source.SupportedHost
		prompted[s.dir] = promptedEntry{name: s.name, source: owner + "/" + repo}
	}

	opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Checking %d installed skill(s) for updates", len(installed)))

	var updates []pendingUpdate
	var pinned []installedSkill

	type repoKey struct{ host, owner, repo string }
	repoSkills := make(map[repoKey][]discovery.Skill)
	repoRefs := make(map[repoKey]*discovery.ResolvedRef)
	repoErrors := make(map[repoKey]bool)

	for _, s := range installed {
		if s.owner == "" || s.repo == "" {
			continue
		}
		if s.pinned != "" && !opts.Unpin {
			pinned = append(pinned, s)
			continue
		}

		key := repoKey{s.repoHost, s.owner, s.repo}

		if repoErrors[key] {
			continue
		}

		// Resolve ref and discover skills once per repo
		if _, ok := repoRefs[key]; !ok {
			resolved, resolveErr := discovery.ResolveRef(apiClient, s.repoHost, s.owner, s.repo, "")
			if resolveErr != nil {
				repoErrors[key] = true
				opts.IO.StopProgressIndicator()
				fmt.Fprintf(opts.IO.ErrOut, "%s Skipping %s: could not resolve %s/%s: %v\n", cs.WarningIcon(), s.name, s.owner, s.repo, resolveErr)
				opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Checking %d installed skill(s) for updates", len(installed)))
				continue
			}
			repoRefs[key] = resolved

			skills, discoverErr := discovery.DiscoverSkills(apiClient, s.repoHost, s.owner, s.repo, resolved.SHA)
			if discoverErr != nil {
				repoErrors[key] = true
				opts.IO.StopProgressIndicator()
				fmt.Fprintf(opts.IO.ErrOut, "%s Skipping %s: %v\n", cs.WarningIcon(), s.name, discoverErr)
				opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Checking %d installed skill(s) for updates", len(installed)))
				continue
			}
			repoSkills[key] = skills
		}

		resolved := repoRefs[key]
		for _, remote := range repoSkills[key] {
			matched := false
			if s.sourcePath != "" {
				matched = remote.Path == s.sourcePath
			} else {
				matched = remote.InstallName() == s.name
			}
			if matched && (remote.TreeSHA != s.treeSHA || opts.Force) {
				updates = append(updates, pendingUpdate{
					local:    s,
					newSHA:   remote.TreeSHA,
					resolved: resolved,
					skill:    remote,
				})
				break
			}
		}
	}

	opts.IO.StopProgressIndicator()

	// Warn about prompted skills that weren't found in the remote repo
	for _, entry := range prompted {
		parts := strings.SplitN(entry.source, "/", 2)
		key := repoKey{source.SupportedHost, parts[0], parts[1]}
		skills, resolved := repoSkills[key]
		if !resolved {
			continue
		}
		found := false
		for _, remote := range skills {
			if remote.InstallName() == entry.name || remote.Name == entry.name {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(opts.IO.ErrOut, "%s Skill %s not found in %s\n", cs.WarningIcon(), entry.name, entry.source)
		}
	}

	for _, s := range pinned {
		fmt.Fprintf(opts.IO.ErrOut, "%s %s is pinned to %s (skipped)\n", cs.Muted("⊘"), s.name, s.pinned)
	}
	for _, name := range noMeta {
		fmt.Fprintf(opts.IO.ErrOut, "%s %s has no GitHub metadata. Reinstall to enable updates\n", cs.WarningIcon(), name)
	}

	if len(updates) == 0 {
		if opts.Force && opts.DryRun {
			fmt.Fprintf(opts.IO.ErrOut, "All skills are up to date. Use --force without --dry-run to re-download anyway.\n")
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "All skills are up to date.\n")
		}
		return nil
	}

	fmt.Fprintf(opts.IO.ErrOut, "\n%d update(s) available:\n", len(updates))
	for _, u := range updates {
		if u.local.treeSHA == u.newSHA {
			fmt.Fprintf(opts.IO.Out, "  %s %s (%s/%s) %s (reinstall) [%s]\n",
				cs.Cyan("•"), u.local.name, u.local.owner, u.local.repo,
				git.ShortSHA(u.newSHA), discovery.ShortRef(u.resolved.Ref))
		} else {
			fmt.Fprintf(opts.IO.Out, "  %s %s (%s/%s) %s > %s [%s]\n",
				cs.Cyan("•"), u.local.name, u.local.owner, u.local.repo,
				cs.Muted(git.ShortSHA(u.local.treeSHA)), git.ShortSHA(u.newSHA),
				discovery.ShortRef(u.resolved.Ref))
		}
	}
	fmt.Fprintln(opts.IO.ErrOut)

	if opts.DryRun {
		return nil
	}

	if !opts.All {
		if !canPrompt {
			return fmt.Errorf("updates available; re-run with --all to apply, or run interactively to confirm")
		}
		confirmed, confirmErr := opts.Prompter.Confirm(fmt.Sprintf("Update %d skill(s)?", len(updates)), true)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			fmt.Fprintf(opts.IO.ErrOut, "Update cancelled.\n")
			return cmdutil.CancelError
		}
	}

	var failed bool
	for _, u := range updates {
		installOpts := &installer.Options{
			Host:      u.local.repoHost,
			Owner:     u.local.owner,
			Repo:      u.local.repo,
			Ref:       u.resolved.Ref,
			SHA:       u.resolved.SHA,
			Skills:    []discovery.Skill{u.skill},
			AgentHost: u.local.host,
			Scope:     u.local.scope,
			GitRoot:   gitRoot,
			HomeDir:   homeDir,
			Client:    apiClient,
		}
		// When updating skills from a custom --dir, host is nil.
		// Use the skill's install root as the target. For namespaced
		// skills (name contains "/"), the dir is two levels below the
		// root instead of one.
		if u.local.host == nil {
			base := filepath.Dir(u.local.dir)
			if strings.Contains(u.local.name, "/") {
				base = filepath.Dir(base)
			}
			installOpts.Dir = base
		}
		_, installErr := installer.Install(installOpts)
		if installErr != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s Failed to update %s: %v\n", cs.FailureIcon(), u.local.name, installErr)
			failed = true
			continue
		}

		// When the install location has changed (e.g. migrating from a
		// namespaced layout to flat), remove the old directory so that the
		// stale copy does not shadow the freshly installed one.
		newDir := filepath.Join(installOpts.Dir, u.skill.Name)
		if installOpts.Dir == "" && u.local.host != nil {
			if d, err := u.local.host.InstallDir(u.local.scope, gitRoot, homeDir); err == nil {
				newDir = filepath.Join(d, u.skill.Name)
			}
		}
		if newDir != "" && u.local.dir != "" && filepath.Clean(newDir) != filepath.Clean(u.local.dir) {
			_ = os.RemoveAll(u.local.dir)
			// Remove the parent if it is now empty (leftover namespace directory).
			parent := filepath.Dir(u.local.dir)
			if entries, readErr := os.ReadDir(parent); readErr == nil && len(entries) == 0 {
				_ = os.Remove(parent)
			}
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "%s Updated %s\n", cs.SuccessIcon(), u.local.name)
		} else {
			fmt.Fprintf(opts.IO.Out, "Updated %s\n", u.local.name)
		}
	}

	if failed {
		return cmdutil.SilentError
	}

	return nil
}

// scanAllAgents walks every registered agent's skill directory (project + user scope) and
// collects installed skills. Shared install roots are scanned only once.
func scanAllAgents(gitRoot, homeDir string) []installedSkill {
	scannedDirs := make(map[string]bool)
	var all []installedSkill

	for i := range registry.Agents {
		host := &registry.Agents[i]
		for _, scope := range []registry.Scope{registry.ScopeProject, registry.ScopeUser} {
			dir, err := host.InstallDir(scope, gitRoot, homeDir)
			if err != nil {
				continue
			}
			if scannedDirs[dir] {
				continue
			}
			scannedDirs[dir] = true
			skills, err := scanInstalledSkills(dir, host, scope)
			if err != nil {
				continue
			}
			all = append(all, skills...)
		}
	}

	return all
}

// scanInstalledSkills reads all SKILL.md files in a skills directory and
// extracts GitHub metadata from their frontmatter. It handles both flat
// layouts ({dir}/{name}/SKILL.md) and namespaced layouts
// ({dir}/{namespace}/{name}/SKILL.md).
func scanInstalledSkills(skillsDir string, host *registry.AgentHost, scope registry.Scope) ([]installedSkill, error) {
	entries, err := os.ReadDir(skillsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read skills directory: %w", err)
	}

	var skills []installedSkill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		// Flat layout: {dir}/{name}/SKILL.md
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		if data, readErr := os.ReadFile(skillFile); readErr == nil {
			if s, ok := parseInstalledSkill(data, e.Name(), filepath.Join(skillsDir, e.Name()), host, scope); ok {
				skills = append(skills, s)
				continue
			}
		}

		// Namespaced layout: {dir}/{namespace}/{name}/SKILL.md
		subEntries, subErr := os.ReadDir(filepath.Join(skillsDir, e.Name()))
		if subErr != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			subSkillFile := filepath.Join(skillsDir, e.Name(), sub.Name(), "SKILL.md")
			if data, readErr := os.ReadFile(subSkillFile); readErr == nil {
				installName := e.Name() + "/" + sub.Name()
				if s, ok := parseInstalledSkill(data, installName, filepath.Join(skillsDir, e.Name(), sub.Name()), host, scope); ok {
					skills = append(skills, s)
				}
			}
		}
	}

	return skills, nil
}

// parseInstalledSkill parses a SKILL.md file and returns an installedSkill.
func parseInstalledSkill(data []byte, name, dir string, host *registry.AgentHost, scope registry.Scope) (installedSkill, bool) {
	result, err := frontmatter.Parse(string(data))
	if err != nil {
		return installedSkill{
			name:        name,
			dir:         dir,
			host:        host,
			scope:       scope,
			metadataErr: fmt.Errorf("invalid SKILL.md: %w", err),
		}, true
	}

	s := installedSkill{
		name:  name,
		dir:   dir,
		host:  host,
		scope: scope,
	}

	if result.Metadata.Meta != nil {
		repoInfo, ok, repoErr := source.ParseMetadataRepo(result.Metadata.Meta)
		if repoErr != nil {
			s.metadataErr = repoErr
		} else if ok {
			if err := source.ValidateSupportedHost(repoInfo.RepoHost()); err != nil {
				s.metadataErr = err
			} else {
				s.repoHost = repoInfo.RepoHost()
				s.owner = repoInfo.RepoOwner()
				s.repo = repoInfo.RepoName()
			}
		}
		s.treeSHA, _ = result.Metadata.Meta["github-tree-sha"].(string)
		s.pinned, _ = result.Metadata.Meta["github-pinned"].(string)
		s.sourcePath, _ = result.Metadata.Meta["github-path"].(string)
	}

	return s, true
}

// promptForSkillOrigin asks the user for the source repository of a skill
// that has no GitHub metadata.
func promptForSkillOrigin(p prompter.Prompter, skillName string) (owner, repo, reason string, ok bool, err error) {
	input, err := p.Input(
		fmt.Sprintf("Repository for %s (owner/repo):", skillName), "")
	if err != nil {
		return "", "", "", false, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", "", false, nil
	}
	r, err := ghrepo.FromFullName(input)
	if err != nil {
		//nolint:nilerr // intentionally converting parse error into a user-facing validation message
		return "", "", fmt.Sprintf("invalid repository %q: expected owner/repo", input), false, nil
	}
	return r.RepoOwner(), r.RepoName(), "", true, nil
}
