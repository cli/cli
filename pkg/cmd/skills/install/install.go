package install

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/installer"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/internal/skills/source"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	// allSkillsKey is the persistent option label for selecting all skills.
	allSkillsKey = "(all skills)"

	// maxSearchResults caps how many skills are shown per search page in
	// interactive selection, keeping the prompt readable.
	maxSearchResults = 30
)

// InstallOptions holds all dependencies and user-provided flags for the install command.
type InstallOptions struct {
	IO         *iostreams.IOStreams
	Telemetry  ghtelemetry.EventRecorder
	HttpClient func() (*http.Client, error)
	Prompter   prompter.Prompter
	GitClient  *git.Client
	Remotes    func() (ghContext.Remotes, error)

	SkillSource  string // owner/repo or local path (when --from-local is set)
	SkillName    string // possibly with @version suffix
	Agent        string
	Scope        string
	ScopeChanged bool // true when --scope was explicitly set
	Pin          string
	Dir          string // overrides --agent and --scope
	Force        bool
	FromLocal    bool // treat SkillSource as a local directory path

	repo      ghrepo.Interface // set when SkillSource is a GitHub repository
	localPath string           // set when FromLocal is true
	version   string           // parsed from SkillName@version
}

// NewCmdInstall creates the "skills install" command.
func NewCmdInstall(f *cmdutil.Factory, telemetry ghtelemetry.CommandRecorder, runF func(*InstallOptions) error) *cobra.Command {
	opts := &InstallOptions{
		IO:         f.IOStreams,
		Telemetry:  telemetry,
		Prompter:   f.Prompter,
		GitClient:  f.GitClient,
		Remotes:    f.Remotes,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "install <repository> [<skill[@version]>] [flags]",
		Short: "Install agent skills from a GitHub repository (preview)",
		Long: heredoc.Docf(`
			Install agent skills from a GitHub repository or local directory into
			your local environment. Skills are placed in a host-specific directory
			at either project scope (inside the current git repository) or user
			scope (in your home directory, available everywhere).

			A wide range of AI coding agents are supported, including GitHub
			Copilot, Claude Code, Cursor, Codex, Gemini CLI, Antigravity, Amp,
			Goose, Junie, OpenCode, Windsurf, and many more.

			Supported %[1]s--agent%[1]s values:

			%[2]s

			Use %[1]s--agent%[1]s and %[1]s--scope%[1]s to control placement, or %[1]s--dir%[1]s for a
			custom directory. The default scope is %[1]sproject%[1]s, and the default
			agent is %[1]sgithub-copilot%[1]s (when running non-interactively).

			At project scope, several agents (including GitHub Copilot, Cursor,
			Codex, Gemini CLI, Antigravity, Amp, Cline, OpenCode, and Warp) share
			the %[1]s.agents/skills%[1]s directory. If you select multiple hosts that
			resolve to the same destination, each skill is installed there only once.

			The first argument is a GitHub repository in %[1]sOWNER/REPO%[1]s format.
			Use %[1]s--from-local%[1]s to install from a local directory instead.
			Local skills are auto-discovered using the same conventions as remote
			repositories, and files are copied (not symlinked) with local-path
			tracking metadata injected into frontmatter.

			Skills are discovered automatically using the %[1]sskills/*/SKILL.md%[1]s convention
			defined by the Agent Skills specification. For more information on the specification, 
			see: https://agentskills.io/specification

			The skill argument can be a name, a namespaced name (%[1]sauthor/skill%[1]s),
			or an exact path within the repository (%[1]sskills/author/skill%[1]s or
			%[1]sskills/author/skill/SKILL.md%[1]s).

			Performance tip: when installing from a large repository with many
			skills, providing an exact path instead of a skill name avoids a
			full tree traversal of the repository, making the install significantly faster.

			When a skill name is provided without a version, the CLI resolves the
			version in this order:

			  1. Latest tagged release in the repository
			  2. Default branch HEAD

			To pin to a specific version, either append %[1]s@VERSION%[1]s to the skill
			name or use the %[1]s--pin%[1]s flag. The version is resolved as a git tag or commit SHA.

			Installed skills have source tracking metadata injected into their
			frontmatter. This metadata identifies the source repository and
			enables %[1]sgh skill update%[1]s to detect changes.

			When run interactively, the command prompts for any missing arguments.
			When run non-interactively, %[1]srepository%[1]s and a skill name are
			required.
		`, "`", registry.AgentHelpList()),
		Example: heredoc.Doc(`
			# Interactive: choose repo, skill, and agent
			$ gh skill install

			# Choose a skill from the repo interactively
			$ gh skill install github/awesome-copilot

			# Install a specific skill
			$ gh skill install github/awesome-copilot git-commit

			# Install a specific version
			$ gh skill install github/awesome-copilot git-commit@v1.2.0

			# Install from a large namespaced repo by path (efficient, skips full discovery)
			$ gh skill install github/awesome-copilot skills/monalisa/code-review

			# Install from a local directory
			$ gh skill install ./my-skills-repo --from-local

			# Install a specific local skill
			$ gh skill install ./my-skills-repo git-commit --from-local

			# Install for Claude Code at user scope
			$ gh skill install github/awesome-copilot git-commit --agent claude-code --scope user

			# Pin to a specific git ref
			$ gh skill install github/awesome-copilot git-commit --pin v2.0.0
		`),
		Aliases: []string{"add"},
		Args:    cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) >= 1 {
				opts.SkillSource = args[0]
			}
			if len(args) >= 2 {
				opts.SkillName = args[1]
			}
			opts.ScopeChanged = cmd.Flags().Changed("scope")

			// Resolve the source type early so installRun can branch directly.
			if opts.FromLocal {
				if opts.SkillSource == "" {
					return cmdutil.FlagErrorf("--from-local requires a directory path argument")
				}
				opts.localPath = opts.SkillSource
			} else if len(args) == 0 && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("must specify a repository to install from")
			}

			if err := cmdutil.MutuallyExclusive("--from-local and --pin cannot be used together", opts.FromLocal, opts.Pin != ""); err != nil {
				return err
			}

			if opts.Pin != "" && opts.SkillName != "" && strings.Contains(opts.SkillName, "@") {
				return cmdutil.FlagErrorf("cannot use --pin with an inline @version in the skill name")
			}

			if runF != nil {
				return runF(opts)
			}
			return installRun(opts)
		},
	}

	agentFlag := cmdutil.StringEnumFlag(cmd, &opts.Agent, "agent", "", "", registry.AgentIDs(), "Target agent")
	agentFlag.Usage = "Target agent (see supported values above)"
	cmdutil.StringEnumFlag(cmd, &opts.Scope, "scope", "", "project", []string{"project", "user"}, "Installation scope")
	cmd.Flags().StringVar(&opts.Pin, "pin", "", "Pin to a specific git tag or commit SHA")
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "Install to a custom directory (overrides --agent and --scope)")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Overwrite existing skills without prompting")
	cmd.Flags().BoolVar(&opts.FromLocal, "from-local", false, "Treat the argument as a local directory path instead of a repository")
	cmdutil.DisableAuthCheckFlag(cmd.Flags().Lookup("from-local"))

	return cmd
}

func installRun(opts *InstallOptions) error {
	cs := opts.IO.ColorScheme()
	canPrompt := opts.IO.CanPrompt()

	if opts.localPath != "" {
		return runLocalInstall(opts)
	}

	repo, repoSource, err := resolveRepoArg(opts.SkillSource, canPrompt, opts.Prompter)
	if err != nil {
		return err
	}
	opts.repo = repo
	opts.SkillSource = repoSource

	parseSkillFromOpts(opts)

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	hostname := opts.repo.RepoHost()
	if err := source.ValidateSupportedHost(hostname); err != nil {
		return err
	}

	// Kick off the visibility fetch in parallel with the install work so
	// the extra API roundtrip doesn't add latency on the critical path.
	// The result is consumed when the telemetry event is emitted below.
	type visResult struct {
		vis discovery.RepoVisibility
		err error
	}
	visCh := make(chan visResult, 1)
	go func() {
		vis, err := discovery.FetchRepoVisibility(apiClient, hostname, opts.repo.RepoOwner(), opts.repo.RepoName())
		visCh <- visResult{vis: vis, err: err}
	}()

	resolved, err := resolveVersion(opts, apiClient, hostname)
	if err != nil {
		return err
	}

	var selectedSkills []discovery.Skill

	if isSkillPath(opts.SkillName) {
		opts.IO.StartProgressIndicatorWithLabel("Looking up skill")
		skill, err := discovery.DiscoverSkillByPath(apiClient, hostname, opts.repo.RepoOwner(), opts.repo.RepoName(), resolved.SHA, opts.SkillName)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
		selectedSkills = []discovery.Skill{*skill}
	} else {
		skills, err := discoverSkills(opts, apiClient, hostname, resolved)
		if err != nil {
			return err
		}

		selectedSkills, err = selectSkillsWithSelector(opts, skills, canPrompt, skillSelector{
			matchByName: matchSkillByName,
			sourceHint:  ghrepo.FullName(opts.repo),
			fetchDescriptions: func() {
				opts.IO.StartProgressIndicatorWithLabel("Fetching skill info")
				discovery.FetchDescriptionsConcurrent(apiClient, hostname, opts.repo.RepoOwner(), opts.repo.RepoName(), skills, nil)
				opts.IO.StopProgressIndicator()
			},
		})
		if err != nil {
			return err
		}
	}

	printPreInstallDisclaimer(opts.IO.ErrOut, cs)

	selectedHosts, err := resolveHosts(opts, canPrompt)
	if err != nil {
		return err
	}

	scope, err := resolveScope(opts, canPrompt)
	if err != nil {
		return err
	}

	gitRoot := installer.ResolveGitRoot(opts.GitClient)
	homeDir := installer.ResolveHomeDir()
	repoSource = ghrepo.FullName(opts.repo)

	plans, err := buildInstallPlans(opts, selectedSkills, selectedHosts, scope, gitRoot, homeDir, canPrompt)
	if err != nil {
		return err
	}

	for _, plan := range plans {
		if len(plans) > 1 {
			fmt.Fprintf(opts.IO.ErrOut, "\nInstalling to %s for %s...\n", friendlyDir(plan.dir), formatPlanHosts(plan.hosts))
		}

		result, err := installer.Install(&installer.Options{
			Host:       hostname,
			Owner:      opts.repo.RepoOwner(),
			Repo:       opts.repo.RepoName(),
			Ref:        resolved.Ref,
			SHA:        resolved.SHA,
			PinnedRef:  opts.Pin,
			Skills:     plan.skills,
			Dir:        plan.dir,
			Client:     apiClient,
			OnProgress: installProgress(opts.IO, len(plan.skills)),
		})

		if result != nil {
			for _, w := range result.Warnings {
				fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", cs.WarningIcon(), w)
			}

			for _, name := range result.Installed {
				fmt.Fprintf(opts.IO.Out, "%s Installed %s (from %s@%s) in %s\n",
					cs.SuccessIcon(), name, repoSource, discovery.ShortRef(resolved.Ref), friendlyDir(result.Dir))
			}

			printFileTree(opts.IO.ErrOut, cs, result.Dir, result.Installed)
			printReviewHint(opts.IO.ErrOut, cs, repoSource, resolved.SHA, result.Installed)
			printHostHints(opts.IO.ErrOut, cs, plan.hosts, result.Installed, result.Dir, gitRoot)
		}

		if err != nil {
			return err
		}
	}

	dims := map[string]string{
		"agent_hosts":     mapAgentHostsToIDs(selectedHosts),
		"skill_host_type": ghinstance.CategorizeHost(opts.repo.RepoHost()),
	}
	select {
	case r := <-visCh:
		if r.err == nil {
			dims["repo_visibility"] = string(r.vis)
			if r.vis == discovery.RepoVisibilityPublic {
				dims["skill_owner"] = opts.repo.RepoOwner()
				dims["skill_repo"] = opts.repo.RepoName()
				dims["skill_names"] = mapSkillsToNames(selectedSkills)
			}
		} else {
			dims["repo_visibility"] = "unknown"
		}
	case <-time.After(visibilityWaitTimeout):
		dims["repo_visibility"] = "unknown"
	}
	opts.Telemetry.Record(ghtelemetry.Event{
		Type:       "skill_install",
		Dimensions: dims,
	})

	return nil
}

// visibilityWaitTimeout is how long to wait at telemetry-emit time for
// the in-flight repo visibility fetch before giving up and emitting
// repo_visibility="unknown". By this point the command has already done
// several serial API calls and (for install) a git sparse-checkout, so
// the fetch has almost always completed; this budget is a short safety
// net for the case where that single REST call has stalled.
const visibilityWaitTimeout = 200 * time.Millisecond

func mapSkillsToNames(skills []discovery.Skill) string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.DisplayName()
	}
	return strings.Join(names, ",")
}

func mapAgentHostsToIDs(hosts []*registry.AgentHost) string {
	agentHostIDs := make([]string, len(hosts))
	for i, h := range hosts {
		agentHostIDs[i] = h.ID
	}
	return strings.Join(agentHostIDs, ",")
}

// runLocalInstall handles installation from a local directory path.
func runLocalInstall(opts *InstallOptions) error {
	cs := opts.IO.ColorScheme()
	canPrompt := opts.IO.CanPrompt()
	sourcePath := opts.localPath
	if sourcePath == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			sourcePath = home
		}
	} else if after, ok := strings.CutPrefix(sourcePath, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			sourcePath = filepath.Join(home, after)
		}
	}

	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("could not resolve path: %w", err)
	}

	opts.IO.StartProgressIndicatorWithLabel("Discovering skills")
	skills, err := discovery.DiscoverLocalSkills(absSource)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if canPrompt {
		fmt.Fprintf(opts.IO.ErrOut, "Found %d skill(s)\n", len(skills))
	}

	selectedSkills, err := selectSkillsWithSelector(opts, skills, canPrompt, skillSelector{
		matchByName: matchLocalSkillByName,
		sourceHint:  absSource,
	})
	if err != nil {
		return err
	}

	printPreInstallDisclaimer(opts.IO.ErrOut, cs)

	selectedHosts, err := resolveHosts(opts, canPrompt)
	if err != nil {
		return err
	}

	scope, err := resolveScope(opts, canPrompt)
	if err != nil {
		return err
	}

	gitRoot := installer.ResolveGitRoot(opts.GitClient)
	homeDir := installer.ResolveHomeDir()

	plans, err := buildInstallPlans(opts, selectedSkills, selectedHosts, scope, gitRoot, homeDir, canPrompt)
	if err != nil {
		return err
	}

	for _, plan := range plans {
		if len(plans) > 1 {
			fmt.Fprintf(opts.IO.ErrOut, "\nInstalling to %s for %s...\n", friendlyDir(plan.dir), formatPlanHosts(plan.hosts))
		}

		result, err := installer.InstallLocal(&installer.LocalOptions{
			SourceDir: absSource,
			Skills:    plan.skills,
			Dir:       plan.dir,
		})
		if err != nil {
			return err
		}

		for _, name := range result.Installed {
			fmt.Fprintf(opts.IO.Out, "Installed %s (from %s) in %s\n",
				name, opts.SkillSource, friendlyDir(result.Dir))
		}

		printFileTree(opts.IO.ErrOut, cs, result.Dir, result.Installed)
		printReviewHint(opts.IO.ErrOut, cs, "", "", result.Installed)
		printHostHints(opts.IO.ErrOut, cs, plan.hosts, result.Installed, result.Dir, gitRoot)
	}

	return nil
}

// isSkillPath returns true if the argument looks like a repo-relative path
// rather than a simple skill name.
func isSkillPath(name string) bool {
	if name == "" {
		return false
	}
	if name == "SKILL.md" || strings.HasSuffix(name, "/SKILL.md") {
		return true
	}
	if strings.HasPrefix(name, "skills/") || strings.HasPrefix(name, "plugins/") {
		return true
	}
	return false
}

func resolveRepoArg(skillSource string, canPrompt bool, p prompter.Prompter) (ghrepo.Interface, string, error) {
	if skillSource == "" {
		if !canPrompt {
			return nil, "", cmdutil.FlagErrorf("must specify a repository to install from")
		}
		repoInput, err := p.Input("Repository (owner/repo):", "")
		if err != nil {
			return nil, "", err
		}
		skillSource = strings.TrimSpace(repoInput)
		if skillSource == "" {
			return nil, "", fmt.Errorf("must specify a repository to install from")
		}
	}
	repo, err := ghrepo.FromFullName(skillSource)
	if err != nil {
		return nil, "", cmdutil.FlagErrorf("invalid repository reference %q: expected OWNER/REPO, HOST/OWNER/REPO, or a full URL", skillSource)
	}
	return repo, skillSource, nil
}

func parseSkillFromOpts(opts *InstallOptions) {
	if opts.SkillName != "" {
		if name, version, ok := cutLast(opts.SkillName, "@"); ok && name != "" {
			opts.version = version
			opts.SkillName = name
			return
		}
	}
	if opts.Pin != "" {
		opts.version = opts.Pin
	}
}

// cutLast splits s around the last occurrence of sep,
// returning the text before and after sep, and whether sep was found.
func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func resolveVersion(opts *InstallOptions, client *api.Client, hostname string) (*discovery.ResolvedRef, error) {
	opts.IO.StartProgressIndicatorWithLabel("Resolving version")
	resolved, err := discovery.ResolveRef(client, hostname, opts.repo.RepoOwner(), opts.repo.RepoName(), opts.version)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return nil, fmt.Errorf("could not resolve version: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "Using ref %s (%s)\n", discovery.ShortRef(resolved.Ref), git.ShortSHA(resolved.SHA))
	return resolved, nil
}

func discoverSkills(opts *InstallOptions, client *api.Client, hostname string, resolved *discovery.ResolvedRef) ([]discovery.Skill, error) {
	opts.IO.StartProgressIndicatorWithLabel("Discovering skills")
	skills, err := discovery.DiscoverSkills(client, hostname, opts.repo.RepoOwner(), opts.repo.RepoName(), resolved.SHA)
	opts.IO.StopProgressIndicator()
	if err != nil {
		var treeTooLarge *discovery.TreeTooLargeError
		if errors.As(err, &treeTooLarge) {
			fmt.Fprintf(opts.IO.ErrOut, "%s\n  Use path-based install instead: gh skill install %s/%s skills/<skill-name>\n",
				err, treeTooLarge.Owner, treeTooLarge.Repo)
			return nil, err
		}
		return nil, err
	}
	logConventions(opts.IO, skills)
	for _, s := range skills {
		if !discovery.IsSpecCompliant(s.Name) {
			fmt.Fprintf(opts.IO.ErrOut, "Warning: skill %q does not follow the agentskills.io naming convention\n", s.DisplayName())
		}
	}
	return skills, nil
}

func logConventions(io *iostreams.IOStreams, skills []discovery.Skill) {
	conventions := make(map[string]int)
	for _, s := range skills {
		conventions[s.Convention]++
	}
	if n, ok := conventions["skills-namespaced"]; ok {
		fmt.Fprintf(io.ErrOut, "Note: found %d namespaced skill(s) in skills/{author}/ directories\n", n)
	}
	if n, ok := conventions["plugins"]; ok {
		fmt.Fprintf(io.ErrOut, "Note: found %d skill(s) using the plugins/ convention\n", n)
	}
	if n, ok := conventions["root"]; ok {
		fmt.Fprintf(io.ErrOut, "Note: found %d skill(s) at the repository root\n", n)
	}
}

// skillSelector holds the callbacks that differ between remote and local skill selection.
type skillSelector struct {
	// matchByName resolves a skill name to matching skills.
	matchByName func(opts *InstallOptions, skills []discovery.Skill) ([]discovery.Skill, error)
	// sourceHint is shown in collision error guidance (e.g. "owner/repo" or "/path/to/skills").
	sourceHint string
	// fetchDescriptions, if non-nil, is called before prompting to pre-populate descriptions.
	fetchDescriptions func()
}

type installPlan struct {
	dir    string
	hosts  []*registry.AgentHost
	skills []discovery.Skill
}

func selectSkillsWithSelector(opts *InstallOptions, skills []discovery.Skill, canPrompt bool, sel skillSelector) ([]discovery.Skill, error) {
	checkCollisions := func(ss []discovery.Skill) error {
		if err := collisionError(ss); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "Hint: install individually using the full name: gh skill install %s namespace/skill-name\n", sel.sourceHint)
			return err
		}
		return nil
	}

	if opts.SkillName != "" {
		return sel.matchByName(opts, skills)
	}

	if !canPrompt {
		return nil, cmdutil.FlagErrorf("must specify a skill name when not running interactively")
	}

	if sel.fetchDescriptions != nil {
		sel.fetchDescriptions()
	}

	tw := opts.IO.TerminalWidth()
	descWidth := tw - 35
	if descWidth < 20 {
		descWidth = 20
	}

	selected, err := opts.Prompter.MultiSelectWithSearch(
		"Select skill(s) to install:",
		"Filter skills",
		nil,
		[]string{allSkillsKey},
		skillSearchFunc(skills, descWidth),
	)
	if err != nil {
		return nil, err
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("must select at least one skill")
	}

	for _, s := range selected {
		if s == allSkillsKey {
			if err := checkCollisions(skills); err != nil {
				return nil, err
			}
			return skills, nil
		}
	}

	result, err := matchSelectedSkills(skills, selected)
	if err != nil {
		return nil, err
	}
	return result, checkCollisions(result)
}

func matchSkillByName(opts *InstallOptions, skills []discovery.Skill) ([]discovery.Skill, error) {
	for _, s := range skills {
		if s.DisplayName() == opts.SkillName {
			return []discovery.Skill{s}, nil
		}
	}

	var matches []discovery.Skill
	for _, s := range skills {
		if s.Name == opts.SkillName {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("skill %q not found in %s", opts.SkillName, ghrepo.FullName(opts.repo))
	case 1:
		return matches, nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.DisplayName()
		}
		return nil, fmt.Errorf(
			"skill name %q is ambiguous, multiple matches found:\n  %s\n  Specify the full name (e.g. %s) to disambiguate",
			opts.SkillName, strings.Join(names, "\n  "), names[0],
		)
	}
}

func matchLocalSkillByName(opts *InstallOptions, skills []discovery.Skill) ([]discovery.Skill, error) {
	for _, s := range skills {
		if s.DisplayName() == opts.SkillName || s.Name == opts.SkillName {
			return []discovery.Skill{s}, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found in local directory", opts.SkillName)
}

// skillSearchFunc returns a search function for MultiSelectWithSearch that
// filters skills by case-insensitive substring match on name and description.
func skillSearchFunc(skills []discovery.Skill, descWidth int) func(string) prompter.MultiSelectSearchResult {
	return func(query string) prompter.MultiSelectSearchResult {
		var matched []discovery.Skill
		if query == "" {
			matched = skills
		} else {
			q := strings.ToLower(query)
			for _, s := range skills {
				if strings.Contains(strings.ToLower(s.DisplayName()), q) ||
					strings.Contains(strings.ToLower(s.Description), q) {
					matched = append(matched, s)
				}
			}
		}

		more := 0
		if len(matched) > maxSearchResults {
			more = len(matched) - maxSearchResults
			matched = matched[:maxSearchResults]
		}

		keys := make([]string, len(matched))
		labels := make([]string, len(matched))
		for i, s := range matched {
			keys[i] = s.DisplayName()
			if s.Description != "" {
				labels[i] = fmt.Sprintf("%s - %s", s.DisplayName(), truncateDescription(s.Description, descWidth))
			} else {
				labels[i] = s.DisplayName()
			}
		}

		return prompter.MultiSelectSearchResult{
			Keys:        keys,
			Labels:      labels,
			MoreResults: more,
		}
	}
}

// matchSelectedSkills maps display names back to skill structs.
func matchSelectedSkills(skills []discovery.Skill, selected []string) ([]discovery.Skill, error) {
	nameSet := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		nameSet[name] = struct{}{}
	}

	var result []discovery.Skill
	for _, s := range skills {
		if _, ok := nameSet[s.DisplayName()]; ok {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no matching skills found")
	}
	return result, nil
}

// collisionError checks for name collisions among the selected skills.
func collisionError(ss []discovery.Skill) error {
	collisions := discovery.FindNameCollisions(ss)
	if len(collisions) == 0 {
		return nil
	}
	return fmt.Errorf("cannot install skills with conflicting names; they would overwrite each other:\n  %s",
		discovery.FormatCollisions(collisions))
}

func resolveHosts(opts *InstallOptions, canPrompt bool) ([]*registry.AgentHost, error) {
	if opts.Agent != "" {
		h, err := registry.FindByID(opts.Agent)
		if err != nil {
			return nil, err
		}
		return []*registry.AgentHost{h}, nil
	}

	if !canPrompt {
		h, err := registry.FindByID(registry.DefaultAgentID)
		if err != nil {
			return nil, err
		}
		return []*registry.AgentHost{h}, nil
	}

	fmt.Fprintln(opts.IO.ErrOut)
	labels := make([]string, len(registry.Agents))
	defaultLabel := ""
	for i, h := range registry.Agents {
		labels[i] = h.Name
		if h.ID == registry.DefaultAgentID {
			defaultLabel = labels[i]
		}
	}
	if defaultLabel == "" {
		defaultLabel = labels[0]
	}
	indices, err := opts.Prompter.MultiSelect("Select target agent(s):", []string{defaultLabel}, labels)
	if err != nil {
		return nil, err
	}

	if len(indices) == 0 {
		return nil, fmt.Errorf("must select at least one target agent")
	}

	selected := make([]*registry.AgentHost, len(indices))
	for i, idx := range indices {
		selected[i] = &registry.Agents[idx]
	}
	return selected, nil
}

func resolveScope(opts *InstallOptions, canPrompt bool) (registry.Scope, error) {
	if opts.Dir != "" {
		return registry.Scope(opts.Scope), nil
	}

	if opts.ScopeChanged || !canPrompt {
		return registry.Scope(opts.Scope), nil
	}

	var repoName string
	if opts.Remotes != nil {
		if remotes, err := opts.Remotes(); err == nil && len(remotes) > 0 {
			repoName = ghrepo.FullName(remotes[0].Repo)
		}
	}
	idx, err := opts.Prompter.Select("Installation scope:", "", registry.ScopeLabels(repoName))
	if err != nil {
		return "", err
	}
	if idx == 0 {
		return registry.ScopeProject, nil
	}
	return registry.ScopeUser, nil
}

func buildInstallPlans(opts *InstallOptions, selectedSkills []discovery.Skill, selectedHosts []*registry.AgentHost, scope registry.Scope, gitRoot, homeDir string, canPrompt bool) ([]installPlan, error) {
	byDir := make(map[string]*installPlan)
	orderedDirs := make([]string, 0, len(selectedHosts))

	for _, host := range selectedHosts {
		targetDir, err := resolveInstallDir(opts, host, scope, gitRoot, homeDir)
		if err != nil {
			return nil, err
		}

		plan, ok := byDir[targetDir]
		if !ok {
			plan = &installPlan{dir: targetDir}
			byDir[targetDir] = plan
			orderedDirs = append(orderedDirs, targetDir)
		}
		plan.hosts = append(plan.hosts, host)
	}

	plans := make([]installPlan, 0, len(orderedDirs))
	for _, dir := range orderedDirs {
		plan := byDir[dir]
		installSkills, err := checkOverwrite(opts, selectedSkills, plan.dir, canPrompt)
		if err != nil {
			return nil, err
		}
		if len(installSkills) == 0 {
			fmt.Fprintf(opts.IO.ErrOut, "No skills to install in %s for %s.\n", friendlyDir(plan.dir), formatPlanHosts(plan.hosts))
			continue
		}
		plan.skills = installSkills
		plans = append(plans, *plan)
	}

	return plans, nil
}

func resolveInstallDir(opts *InstallOptions, host *registry.AgentHost, scope registry.Scope, gitRoot, homeDir string) (string, error) {
	if opts.Dir != "" {
		return opts.Dir, nil
	}
	return host.InstallDir(scope, gitRoot, homeDir)
}

func formatPlanHosts(hosts []*registry.AgentHost) string {
	names := make([]string, len(hosts))
	for i, host := range hosts {
		names[i] = host.Name
	}
	return strings.Join(names, ", ")
}

func truncateDescription(s string, maxWidth int) string {
	return text.Truncate(maxWidth, text.RemoveExcessiveWhitespace(s))
}

func checkOverwrite(opts *InstallOptions, skills []discovery.Skill, targetDir string, canPrompt bool) ([]discovery.Skill, error) {
	var existing, fresh []discovery.Skill
	for _, s := range skills {
		dir := filepath.Join(targetDir, filepath.FromSlash(s.InstallName()))
		if _, err := os.Stat(dir); err == nil {
			existing = append(existing, s)
		} else {
			fresh = append(fresh, s)
		}
	}

	if len(existing) == 0 {
		return skills, nil
	}

	if opts.Force {
		return skills, nil
	}

	if !canPrompt {
		names := make([]string, len(existing))
		for i, s := range existing {
			names[i] = s.DisplayName()
		}
		return nil, fmt.Errorf("skills already installed: %s (use --force to overwrite)", strings.Join(names, ", "))
	}

	var confirmed []discovery.Skill
	for _, s := range existing {
		prompt := existingSkillPrompt(targetDir, s)
		ok, err := opts.Prompter.Confirm(prompt, false)
		if err != nil {
			return nil, err
		}
		if ok {
			confirmed = append(confirmed, s)
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "Skipping %s\n", s.DisplayName())
		}
	}

	return append(fresh, confirmed...), nil
}

func existingSkillPrompt(targetDir string, incoming discovery.Skill) string {
	skillFile := filepath.Join(targetDir, filepath.FromSlash(incoming.InstallName()), "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return fmt.Sprintf("Skill %q already exists. Overwrite?", incoming.DisplayName())
	}

	result, err := frontmatter.Parse(string(data))
	if err != nil || result.Metadata.Meta == nil {
		return fmt.Sprintf("Skill %q already exists. Overwrite?", incoming.DisplayName())
	}

	repoInfo, _, err := source.ParseMetadataRepo(result.Metadata.Meta)
	ref, _ := result.Metadata.Meta["github-ref"].(string)
	if err != nil {
		return fmt.Sprintf("Skill %q already exists. Overwrite?", incoming.DisplayName())
	}

	if repoInfo != nil {
		sourceName := ghrepo.FullName(repoInfo)
		if ref != "" {
			sourceName += "@" + ref
		}
		return fmt.Sprintf("Skill %q already installed from %s. Overwrite?", incoming.DisplayName(), sourceName)
	}

	return fmt.Sprintf("Skill %q already exists. Overwrite?", incoming.DisplayName())
}

const installProgressLabel = "Downloading skill files"

func installProgress(io *iostreams.IOStreams, total int) func(done, total int) {
	if total <= 0 {
		return nil
	}
	return func(done, total int) {
		if done == 0 {
			io.StartProgressIndicatorWithLabel(installProgressLabel)
		} else if done >= total {
			io.StopProgressIndicator()
		}
	}
}

func friendlyDir(dir string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return filepath.Base(dir)
			}
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "~/" + rel
		}
	}
	return dir
}

// printFileTree renders a text tree of the on-disk contents of each skill directory.
func printFileTree(w io.Writer, cs *iostreams.ColorScheme, dir string, skillNames []string) {
	if len(skillNames) == 0 {
		return
	}
	fmt.Fprintln(w)
	for _, name := range skillNames {
		skillDir := filepath.Join(dir, filepath.FromSlash(name))
		fmt.Fprintf(w, "  %s\n", cs.Bold(name+"/"))
		printTreeDir(w, cs, skillDir, "  ")
	}
}

func printTreeDir(w io.Writer, cs *iostreams.ColorScheme, dir, indent string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(w, "%s%s\n", indent, cs.Muted("(could not read directory)"))
		return
	}
	for i, entry := range entries {
		isLast := i == len(entries)-1
		connector := "├── "
		childIndent := "│   "
		if isLast {
			connector = "└── "
			childIndent = "    "
		}
		name := entry.Name()
		if entry.IsDir() {
			fmt.Fprintf(w, "%s%s%s\n", indent, cs.Muted(connector), cs.Bold(name+"/"))
			printTreeDir(w, cs, filepath.Join(dir, name), indent+cs.Muted(childIndent))
		} else {
			fmt.Fprintf(w, "%s%s%s\n", indent, cs.Muted(connector), name)
		}
	}
}

// printPreInstallDisclaimer prints a warning that installed skills are unverified
// and should be inspected before use.
func printPreInstallDisclaimer(w io.Writer, cs *iostreams.ColorScheme) {
	fmt.Fprintf(w, "\n%s Skills are not verified by GitHub and may contain prompt injections, hidden instructions, or malicious scripts. Always review skill contents before use.\n\n", cs.WarningIcon())
}

// printReviewHint warns the user to review installed skills and suggests preview commands.
// When sha is non-empty the suggested commands include @SHA so the user previews
// exactly the version that was installed.
func printReviewHint(w io.Writer, cs *iostreams.ColorScheme, repo, sha string, skillNames []string) {
	if len(skillNames) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s Skills may contain prompt injections or malicious scripts.\n", cs.WarningIcon())
	if repo == "" {
		fmt.Fprintln(w, "  Review the installed files before use.")
		return
	}
	fmt.Fprintln(w, "  Review installed content before use:")
	fmt.Fprintln(w)
	for _, name := range skillNames {
		if sha != "" {
			fmt.Fprintf(w, "    gh skill preview %s %s@%s\n", repo, name, sha)
		} else {
			fmt.Fprintf(w, "    gh skill preview %s %s\n", repo, name)
		}
	}
	fmt.Fprintln(w)
}

// printHostHints prints any agent-specific post-install guidance for the
// hosts that were installed to. Most agents need no extra steps; this is
// currently used for Kiro CLI, which requires skills to be registered as
// resources on a custom agent. The path in the example is derived from
// the actual install directory so it matches the chosen scope or --dir.
func printHostHints(w io.Writer, cs *iostreams.ColorScheme, hosts []*registry.AgentHost, installed []string, installDir, gitRoot string) {
	if len(installed) == 0 {
		return
	}
	for _, h := range hosts {
		if h.ID == "kiro-cli" {
			fmt.Fprintln(w)
			fmt.Fprint(w, heredoc.Docf(`
				%s Kiro CLI: register these skills on a custom agent by adding them to
				  .kiro/agents/<agent>.json under "resources", for example:

				    {
				      "resources": ["skill://%s/**/SKILL.md"]
				    }
			`, cs.WarningIcon(), kiroResourcePath(installDir, gitRoot)))
			fmt.Fprintln(w)
			return
		}
	}
}

// kiroResourcePath returns a slash-separated path suitable for use in the
// "resources" field of a Kiro agent config. When the install directory is
// inside the current git repository the path is made relative to the repo
// root so the example works for project-scoped agent configs; otherwise
// the absolute install path is used (e.g. for --scope user or --dir).
func kiroResourcePath(installDir, gitRoot string) string {
	if gitRoot != "" && installDir != "" {
		if rel, err := filepath.Rel(gitRoot, installDir); err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(installDir)
}
