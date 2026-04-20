package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/internal/skills/source"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// PublishOptions holds all dependencies and user-provided flags for the publish command.
type PublishOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	Prompter   prompter.Prompter
	GitClient  *git.Client

	Dir    string
	Fix    bool
	DryRun bool
	Tag    string

	host string // resolved from config in production
}

// publishDiagnostic is a single validation finding.
type publishDiagnostic struct {
	skill    string // empty for repo-level issues
	severity string // "error", "warning", "fixed", or "info"
	message  string
}

// repoTopicsResponse is the response from the repo topics API.
type repoTopicsResponse struct {
	Names []string `json:"names"`
}

// tagEntry is a single tag from the tags list API.
type tagEntry struct {
	Name string `json:"name"`
}

// rulesetsResponse is a single ruleset from the rulesets API.
type rulesetsResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	Enforcement string `json:"enforcement"`
}

// securityAnalysis represents the security_and_analysis field from the repo API.
type securityAnalysis struct {
	AdvancedSecurity             *securityFeature `json:"advanced_security"`
	SecretScanning               *securityFeature `json:"secret_scanning"`
	SecretScanningPushProtection *securityFeature `json:"secret_scanning_push_protection"`
}

type securityFeature struct {
	Status string `json:"status"`
}

// repoSecurityResponse is the subset of repo API we need for security checks.
type repoSecurityResponse struct {
	SecurityAndAnalysis *securityAnalysis `json:"security_and_analysis"`
}

// NewCmdPublish creates the "skills publish" command.
func NewCmdPublish(f *cmdutil.Factory, runF func(*PublishOptions) error) *cobra.Command {
	opts := &PublishOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
		GitClient:  f.GitClient,
	}

	cmd := &cobra.Command{
		Use:   "publish [<directory>] [flags]",
		Short: "Validate and publish skills to a GitHub repository (preview)",
		Long: heredoc.Docf(`
			Validate a local repository's skills against the Agent Skills specification
			and publish them by creating a GitHub release.

			Skills are discovered using the same conventions as install:

			  - %[1]sskills/*/SKILL.md%[1]s
			  - %[1]sskills/{scope}/*/SKILL.md%[1]s
			  - %[1]s*/SKILL.md%[1]s (root-level)
			  - %[1]splugins/{scope}/skills/*/SKILL.md%[1]s

			Validation checks include:

			  - Skill names match the strict agentskills.io naming rules
			  - Each skill name matches its directory name
			  - Required frontmatter fields (name, description) are present
			  - allowed-tools is a string, not an array
			  - Install metadata (%[1]smetadata.github-*%[1]s) is stripped if present

			After validation passes, publish will interactively guide you through:

			  - Adding the %[1]sagent-skills%[1]s topic to the repository
			  - Choosing a version tag (semver recommended)
			  - Creating a GitHub release with auto-generated notes

			Use %[1]s--dry-run%[1]s to validate without publishing.
			Use %[1]s--tag%[1]s to publish non-interactively with a specific tag.
			Use %[1]s--fix%[1]s to automatically strip install metadata from committed files
			without publishing. Review and commit the changes, then run publish again.
		`, "`"),
		Example: heredoc.Doc(`
			# Validate and publish interactively
			$ gh skill publish

			# Publish with a specific tag (non-interactive)
			$ gh skill publish --tag v1.0.0

			# Validate only (no publish)
			$ gh skill publish --dry-run

			# Strip install metadata without publishing
			$ gh skill publish --fix
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Dir = args[0]
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--fix` or `--dry-run`", opts.Fix, opts.DryRun); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return publishRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Fix, "fix", false, "Auto-fix issues where possible without publishing (e.g. strip install metadata)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Validate without publishing")
	cmd.Flags().StringVar(&opts.Tag, "tag", "", "Version tag for the release (e.g. v1.0.0)")

	return cmd
}

func publishRun(opts *PublishOptions) error {
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine working directory: %w", err)
		}
	}

	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("could not resolve path: %w", err)
	}

	canPrompt := opts.IO.CanPrompt()

	// Client initialization is deferred until after local validation so that
	// simple errors (missing skills/, bad SKILL.md, etc.) are reported
	// without requiring an HTTP client.
	var client *api.Client
	host := opts.host

	var diagnostics []publishDiagnostic

	skills, err := discovery.DiscoverLocalSkills(dir)
	if err != nil {
		return err
	}

	for _, skill := range skills {
		dirName := path.Base(skill.Path)
		skillPath := filepath.Join(dir, filepath.FromSlash(skill.Path), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "error",
				message:  "missing SKILL.md file",
			})
			continue
		}

		result, err := frontmatter.Parse(string(content))
		if err != nil {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "error",
				message:  fmt.Sprintf("invalid frontmatter YAML: %s", err),
			})
			continue
		}

		// Validate name field exists
		if result.Metadata.Name == "" {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "error",
				message:  "missing required field: name",
			})
		} else {
			// Validate name matches directory
			if result.Metadata.Name != dirName {
				diagnostics = append(diagnostics, publishDiagnostic{
					skill:    skill.DisplayName(),
					severity: "error",
					message:  fmt.Sprintf("name %q does not match directory name %q", result.Metadata.Name, dirName),
				})
			}

			// Validate name is spec-compliant
			if !discovery.IsSpecCompliant(result.Metadata.Name) {
				diagnostics = append(diagnostics, publishDiagnostic{
					skill:    skill.DisplayName(),
					severity: "error",
					message:  fmt.Sprintf("name %q does not follow agentskills.io naming convention (lowercase alphanumeric + hyphens)", result.Metadata.Name),
				})
			}
		}

		// Validate description field exists
		if result.Metadata.Description == "" {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "error",
				message:  "missing required field: description",
			})
		} else if len(result.Metadata.Description) > 1024 {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "warning",
				message:  fmt.Sprintf("description is %d chars (recommended max: 1024)", len(result.Metadata.Description)),
			})
		}

		// Validate allowed-tools is string, not array
		if raw, ok := result.RawYAML["allowed-tools"]; ok {
			if _, isSlice := raw.([]interface{}); isSlice {
				diagnostics = append(diagnostics, publishDiagnostic{
					skill:    skill.DisplayName(),
					severity: "error",
					message:  "allowed-tools must be a string (space-delimited), not an array",
				})
			}
		}

		// Check for install metadata that should be stripped
		if meta, ok := result.RawYAML["metadata"].(map[string]interface{}); ok {
			githubKeys := findGitHubMetadataKeys(meta)
			if len(githubKeys) > 0 {
				if opts.Fix {
					fixed, fixErr := stripGitHubMetadata(string(content))
					if fixErr != nil {
						diagnostics = append(diagnostics, publishDiagnostic{
							skill:    skill.DisplayName(),
							severity: "error",
							message:  fmt.Sprintf("could not strip install metadata: %s", fixErr),
						})
					} else if writeErr := os.WriteFile(skillPath, []byte(fixed), 0o644); writeErr != nil {
						diagnostics = append(diagnostics, publishDiagnostic{
							skill:    skill.DisplayName(),
							severity: "error",
							message:  fmt.Sprintf("could not write fixed SKILL.md: %s", writeErr),
						})
					} else {
						diagnostics = append(diagnostics, publishDiagnostic{
							skill:    skill.DisplayName(),
							severity: "fixed",
							message:  fmt.Sprintf("stripped install metadata: %s", strings.Join(githubKeys, ", ")),
						})
					}
				} else {
					diagnostics = append(diagnostics, publishDiagnostic{
						skill:    skill.DisplayName(),
						severity: "error",
						message:  fmt.Sprintf("contains install metadata that must be stripped: %s (use --fix)", strings.Join(githubKeys, ", ")),
					})
				}
			}
		}

		// Recommended: license field
		if result.Metadata.License == "" {
			if _, ok := result.RawYAML["license"]; !ok {
				diagnostics = append(diagnostics, publishDiagnostic{
					skill:    skill.DisplayName(),
					severity: "warning",
					message:  "recommended field missing: license",
				})
			}
		}

		// Recommended: body length
		bodyLines := strings.Count(result.Body, "\n") + 1
		if bodyLines > 500 {
			diagnostics = append(diagnostics, publishDiagnostic{
				skill:    skill.DisplayName(),
				severity: "warning",
				message:  fmt.Sprintf("skill body is %d lines (recommended max: 500 for efficient context)", bodyLines),
			})
		}
	}

	// Check for installed skill directories that should be gitignored
	installedDirDiags := checkInstalledSkillDirs(opts.GitClient, dir)
	diagnostics = append(diagnostics, installedDirDiags...)

	// Remote repository checks (best-effort)
	repoInfo, remoteErr := detectGitHubRemote(opts.GitClient, dir)
	if remoteErr != nil {
		return remoteErr
	}
	owner, repo := "", ""
	if repoInfo != nil {
		owner = repoInfo.Repo.RepoOwner()
		repo = repoInfo.Repo.RepoName()
	}

	hasTopic := false
	var existingTags []tagEntry
	if owner != "" && repo != "" {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		client = api.NewClientFromHTTP(httpClient)

		if host == "" && repoInfo != nil {
			host = repoInfo.Repo.RepoHost()
		}
		if host == "" {
			cfg, err := opts.Config()
			if err != nil {
				return err
			}
			host, _ = cfg.Authentication().DefaultHost()
		}
		if err := source.ValidateSupportedHost(host); err != nil {
			return err
		}

		// Security and ruleset checks (advisory, always shown)
		var skillAbsDirs []string
		for _, skill := range skills {
			skillAbsDirs = append(skillAbsDirs, filepath.Join(dir, filepath.FromSlash(skill.Path)))
		}
		securityDiags := checkSecuritySettings(client, host, owner, repo, skillAbsDirs)
		diagnostics = append(diagnostics, securityDiags...)

		rulesetDiags := checkTagProtection(client, host, owner, repo)
		diagnostics = append(diagnostics, rulesetDiags...)

		// Check topic (needed for publish flow, not a blocking error)
		hasTopic = repoHasTopic(client, host, owner, repo)

		// Fetch existing tags (needed for version suggestion)
		existingTags = fetchTags(client, host, owner, repo)
	} else {
		diagnostics = append(diagnostics, detectMissingRepoDiagnostic(opts.GitClient, dir)...)
	}

	// Render diagnostics
	errors, warnings, fixes := 0, 0, 0
	for _, d := range diagnostics {
		switch d.severity {
		case "error":
			errors++
		case "warning":
			warnings++
		case "fixed":
			fixes++
		}
	}

	if canPrompt {
		renderDiagnosticsTTY(opts, len(skills), diagnostics, errors, warnings, fixes, owner, repo)
	} else {
		renderDiagnosticsPlain(opts, diagnostics, errors, warnings)
	}

	if errors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", errors)
	}

	// --- Publish flow ---
	if opts.DryRun {
		fmt.Fprintf(opts.IO.ErrOut, "\nDry run complete. Use without --dry-run to publish.\n")
		return nil
	}

	if opts.Fix {
		if fixes > 0 {
			fmt.Fprintf(opts.IO.ErrOut, "\nFixed %d file(s). Review and commit the changes, then run %s to publish.\n", fixes, "gh skill publish")
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "\nNo issues to fix.\n")
		}
		return nil
	}

	if owner == "" || repo == "" {
		fmt.Fprintf(opts.IO.ErrOut, "\nValidation passed. Set up a GitHub remote to publish.\n")
		return nil
	}

	if !canPrompt && opts.Tag == "" {
		fmt.Fprintf(opts.IO.ErrOut, "\nValidation passed. Use --tag to publish non-interactively.\n")
		return nil
	}

	fmt.Fprintf(opts.IO.ErrOut, "\nPublishing to %s/%s...\n\n", owner, repo)

	return runPublishRelease(opts, client, host, owner, repo, dir, repoInfo.RemoteName, hasTopic, existingTags)
}

// repoHasTopic checks whether the repo has the agent-skills topic.
func repoHasTopic(client *api.Client, host, owner, repo string) bool {
	if client == nil {
		return false
	}
	apiPath := fmt.Sprintf("repos/%s/%s/topics", owner, repo)
	var resp repoTopicsResponse
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return false
	}
	for _, t := range resp.Names {
		if t == "agent-skills" {
			return true
		}
	}
	return false
}

// fetchTags returns the most recent tags from the repo.
func fetchTags(client *api.Client, host, owner, repo string) []tagEntry {
	if client == nil {
		return nil
	}
	apiPath := fmt.Sprintf("repos/%s/%s/tags?per_page=10", owner, repo)
	var tags []tagEntry
	if err := client.REST(host, "GET", apiPath, nil, &tags); err != nil {
		return nil
	}
	return tags
}

// runPublishRelease handles the interactive publish flow: topic, tag, release, immutability.
func runPublishRelease(opts *PublishOptions, client *api.Client, host, owner, repo, dir, remoteName string, hasTopic bool, existingTags []tagEntry) error {
	cs := opts.IO.ColorScheme()
	canPrompt := opts.IO.CanPrompt()

	// Add topic if missing
	if !hasTopic {
		addTopic := true
		if canPrompt {
			var err error
			addTopic, err = opts.Prompter.Confirm(
				fmt.Sprintf("Add \"agent-skills\" topic to %s/%s? (required for discoverability)", owner, repo), true)
			if err != nil {
				return err
			}
		}
		if addTopic {
			if err := addAgentSkillsTopic(client, host, owner, repo); err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s Could not add topic: %v\n", cs.WarningIcon(), err)
				fmt.Fprintf(opts.IO.ErrOut, "  Add it manually: gh repo edit %s/%s --add-topic agent-skills\n", owner, repo)
			} else {
				fmt.Fprintf(opts.IO.Out, "%s Added \"agent-skills\" topic\n", cs.SuccessIcon())
			}
		}
	}

	// Push unpushed commits (like gh pr create)
	if err := ensurePushed(opts, dir, remoteName); err != nil {
		return err
	}

	// Determine tag
	tag := opts.Tag
	if tag == "" {
		suggested := "v1.0.0"
		if len(existingTags) > 0 {
			if next := suggestNextTag(existingTags[0].Name); next != "" {
				suggested = next
			}
		}

		if canPrompt {
			strategies := []string{
				fmt.Sprintf("Semver (recommended): %s", suggested),
				"Custom tag",
			}
			idx, err := opts.Prompter.Select("Tagging strategy:", "", strategies)
			if err != nil {
				return err
			}

			if idx == 0 {
				tag = suggested
				edited, err := opts.Prompter.Input(fmt.Sprintf("Version tag [%s]:", suggested), suggested)
				if err != nil {
					return err
				}
				if edited != "" {
					tag = edited
				}
			} else {
				custom, err := opts.Prompter.Input("Tag:", "")
				if err != nil {
					return err
				}
				if custom == "" {
					return fmt.Errorf("tag is required")
				}
				tag = custom
			}
		} else {
			return fmt.Errorf("--tag is required for non-interactive publish")
		}
	}

	// Validate tag doesn't already exist
	for _, t := range existingTags {
		if t.Name == tag {
			return fmt.Errorf("tag %s already exists; choose a different version", tag)
		}
	}

	// Offer to enable immutable releases
	immutableEnabled := checkImmutableReleases(client, host, owner, repo)
	if !immutableEnabled && canPrompt {
		enableImmutable, err := opts.Prompter.Confirm(
			"Enable immutable releases? (prevents tampering with published releases)", true)
		if err != nil {
			return err
		}
		if enableImmutable {
			if err := enableImmutableReleases(client, host, owner, repo); err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s Could not enable immutable releases: %v\n", cs.WarningIcon(), err)
				fmt.Fprintf(opts.IO.ErrOut, "  Enable manually in Settings > General > Releases\n")
			} else {
				fmt.Fprintf(opts.IO.Out, "%s Enabled immutable releases\n", cs.SuccessIcon())
			}
		}
	}

	// Inform if not on default branch
	var currentBranch string
	if opts.GitClient != nil {
		branchGitClient := opts.GitClient.Copy()
		branchGitClient.RepoDir = dir
		if b, err := branchGitClient.CurrentBranch(context.Background()); err == nil {
			currentBranch = b
		}
	}
	defaultBranch := detectDefaultBranch(client, host, owner, repo)
	if currentBranch != "" && defaultBranch != "" && currentBranch != defaultBranch {
		fmt.Fprintf(opts.IO.ErrOut, "%s Publishing from branch %q (default is %q)\n", cs.WarningIcon(), currentBranch, defaultBranch)
	}

	// Confirm and create release
	if canPrompt {
		confirmed, err := opts.Prompter.Confirm(
			fmt.Sprintf("Create release %s with auto-generated notes?", tag), true)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintf(opts.IO.ErrOut, "Publish cancelled.\n")
			return cmdutil.CancelError
		}
	}

	// Create release via REST API
	releaseBody := map[string]interface{}{
		"tag_name":               tag,
		"generate_release_notes": true,
	}
	if currentBranch != "" {
		releaseBody["target_commitish"] = currentBranch
	}
	releaseJSON, err := json.Marshal(releaseBody)
	if err != nil {
		return fmt.Errorf("failed to serialize release request: %w", err)
	}

	releasePath := fmt.Sprintf("repos/%s/%s/releases", owner, repo)
	var releaseResp struct {
		HTMLURL string `json:"html_url"`
	}
	if err := client.REST(host, "POST", releasePath, bytes.NewReader(releaseJSON), &releaseResp); err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	fmt.Fprintf(opts.IO.Out, "%s Published %s\n", cs.SuccessIcon(), tag)
	fmt.Fprintf(opts.IO.Out, "%s Install with: gh skill install %s/%s\n", cs.SuccessIcon(), owner, repo)
	fmt.Fprintf(opts.IO.Out, "%s Pin with:     gh skill install %s/%s <skill> --pin %s\n", cs.SuccessIcon(), owner, repo, tag)

	return nil
}

// ensurePushed checks whether the current branch has unpushed commits and
// pushes them automatically, consistent with how gh pr create behaves.
func ensurePushed(opts *PublishOptions, dir, remoteName string) error {
	if opts.GitClient == nil {
		return nil
	}

	cs := opts.IO.ColorScheme()
	gitClient := opts.GitClient.Copy()
	gitClient.RepoDir = dir

	ctx := context.Background()
	currentBranch, err := gitClient.CurrentBranch(ctx)
	if err != nil {
		return nil //nolint:nilerr // not on a branch (detached HEAD); skip push check
	}

	// Count commits ahead of the push target (remote tracking branch).
	// If the branch has no upstream, rev-list will fail; we treat that as
	// "everything is unpushed" and push the whole branch.
	unpushed := 0
	revCmd, err := gitClient.Command(ctx, "rev-list", "--count", "@{push}..HEAD")
	if err != nil {
		return fmt.Errorf("could not check unpushed commits: %w", err)
	}
	out, revErr := revCmd.Output()
	if revErr != nil {
		// @{push} not resolvable; branch has never been pushed
		unpushed = -1
	} else {
		n, parseErr := strconv.Atoi(strings.TrimSpace(string(out)))
		if parseErr != nil {
			return fmt.Errorf("could not parse unpushed commit count: %w", parseErr)
		}
		unpushed = n
	}

	if unpushed == 0 {
		return nil
	}

	ref := fmt.Sprintf("HEAD:refs/heads/%s", currentBranch)
	fmt.Fprintf(opts.IO.ErrOut, "Pushing %s to %s...\n", currentBranch, remoteName)
	if err := gitClient.Push(ctx, remoteName, ref); err != nil {
		return fmt.Errorf("failed to push branch %s: %w", currentBranch, err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Pushed %s to %s\n", cs.SuccessIcon(), currentBranch, remoteName)

	return nil
}

// detectDefaultBranch returns the default branch of the remote repo via the API.
func detectDefaultBranch(client *api.Client, host, owner, repo string) string {
	if client == nil {
		return ""
	}
	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := client.REST(host, "GET", fmt.Sprintf("repos/%s/%s", owner, repo), nil, &result); err != nil {
		return ""
	}
	return result.DefaultBranch
}

// addAgentSkillsTopic adds the "agent-skills" topic to the repo, preserving existing topics.
func addAgentSkillsTopic(client *api.Client, host, owner, repo string) error {
	apiPath := fmt.Sprintf("repos/%s/%s/topics", owner, repo)

	// Fetch existing topics
	var resp repoTopicsResponse
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return fmt.Errorf("could not fetch existing topics: %w", err)
	}

	// Deduplicate: only add if not already present
	for _, t := range resp.Names {
		if t == "agent-skills" {
			return nil
		}
	}

	topics := append(resp.Names, "agent-skills")
	topicsJSON, err := json.Marshal(map[string][]string{"names": topics})
	if err != nil {
		return fmt.Errorf("could not serialize topics: %w", err)
	}
	return client.REST(host, "PUT", apiPath, bytes.NewReader(topicsJSON), nil)
}

// checkImmutableReleases checks if immutable releases are enabled for the repo.
func checkImmutableReleases(client *api.Client, host, owner, repo string) bool {
	if client == nil {
		return false
	}
	apiPath := fmt.Sprintf("repos/%s/%s/immutable-releases", owner, repo)
	var resp struct {
		Enabled bool `json:"enabled"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return false
	}
	return resp.Enabled
}

// enableImmutableReleases enables immutable releases for the repo.
func enableImmutableReleases(client *api.Client, host, owner, repo string) error {
	apiPath := fmt.Sprintf("repos/%s/%s/immutable-releases", owner, repo)
	body := bytes.NewReader([]byte(`{"enabled":true}`))
	return client.REST(host, "PATCH", apiPath, body, nil)
}

// checkTagProtection checks whether tag protection rulesets are enabled.
func checkTagProtection(client *api.Client, host, owner, repo string) []publishDiagnostic {
	if client == nil {
		return nil
	}
	apiPath := fmt.Sprintf("repos/%s/%s/rulesets", owner, repo)
	var rulesets []rulesetsResponse
	if err := client.REST(host, "GET", apiPath, nil, &rulesets); err != nil {
		return nil
	}

	for _, rs := range rulesets {
		if rs.Target == "tag" && rs.Enforcement == "active" {
			return nil
		}
	}

	return []publishDiagnostic{{
		severity: "warning",
		message:  "no active tag protection rulesets found. Consider protecting tags to ensure immutable releases (Settings > Rules > Rulesets)",
	}}
}

// checkSecuritySettings checks whether recommended security features are enabled.
func checkSecuritySettings(client *api.Client, host, owner, repo string, skillDirs []string) []publishDiagnostic {
	if client == nil {
		return nil
	}
	apiPath := fmt.Sprintf("repos/%s/%s", owner, repo)
	var resp repoSecurityResponse
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return nil
	}

	if resp.SecurityAndAnalysis == nil {
		return nil
	}

	var diagnostics []publishDiagnostic
	sa := resp.SecurityAndAnalysis

	if sa.SecretScanning == nil || sa.SecretScanning.Status != "enabled" {
		diagnostics = append(diagnostics, publishDiagnostic{
			severity: "warning",
			message:  "secret scanning is not enabled. Recommended to prevent accidental credential exposure (gh repo edit --enable-secret-scanning)",
		})
	}

	if sa.SecretScanningPushProtection == nil || sa.SecretScanningPushProtection.Status != "enabled" {
		diagnostics = append(diagnostics, publishDiagnostic{
			severity: "warning",
			message:  "secret scanning push protection is not enabled. Blocks pushes containing secrets (gh repo edit --enable-secret-scanning-push-protection)",
		})
	}

	hasCode, hasManifests := detectCodeAndManifests(skillDirs)

	if hasCode {
		alertsPath := fmt.Sprintf("repos/%s/%s/code-scanning/alerts?per_page=1&state=open", owner, repo)
		if err := client.REST(host, "GET", alertsPath, nil, new([]interface{})); err != nil {
			diagnostics = append(diagnostics, publishDiagnostic{
				severity: "info",
				message:  "skills include code files but code scanning does not appear to be configured (Settings > Code security > Code scanning)",
			})
		}
	}

	if hasManifests {
		dependabotPath := fmt.Sprintf("repos/%s/%s/vulnerability-alerts", owner, repo)
		if err := client.REST(host, "GET", dependabotPath, nil, nil); err != nil {
			diagnostics = append(diagnostics, publishDiagnostic{
				severity: "info",
				message:  "skills include dependency manifests but Dependabot alerts do not appear to be enabled (Settings > Code security > Dependabot)",
			})
		}
	}

	return diagnostics
}

// codeExtensions are file extensions that indicate code is present.
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".rb": true,
	".rs": true, ".java": true, ".cs": true, ".sh": true, ".bash": true,
	".zsh": true, ".ps1": true, ".swift": true, ".kt": true, ".c": true,
	".cpp": true, ".h": true, ".php": true, ".pl": true, ".lua": true,
}

// manifestFiles are dependency manifest filenames.
var manifestFiles = map[string]bool{
	"package.json": true, "package-lock.json": true, "yarn.lock": true,
	"go.mod": true, "go.sum": true, "Cargo.toml": true, "Cargo.lock": true,
	"requirements.txt": true, "Pipfile": true, "Pipfile.lock": true,
	"pyproject.toml": true, "poetry.lock": true, "Gemfile": true,
	"Gemfile.lock": true, "pom.xml": true, "build.gradle": true,
	"composer.json": true, "composer.lock": true,
}

// detectCodeAndManifests walks the skill directories looking for code files
// and dependency manifests.
func detectCodeAndManifests(skillDirs []string) (hasCode, hasManifests bool) {
	for _, dir := range skillDirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			ext := filepath.Ext(info.Name())
			if codeExtensions[ext] {
				hasCode = true
			}
			if manifestFiles[info.Name()] {
				hasManifests = true
			}
			if hasCode && hasManifests {
				// Stop walking this skill directory early; the outer loop
				// continues to process remaining skill directories.
				return filepath.SkipAll
			}
			return nil
		})
		if hasCode && hasManifests {
			return
		}
	}
	return
}

// checkInstalledSkillDirs warns when agent host skill directories exist
// in the repo and are not gitignored.
func checkInstalledSkillDirs(gitClient *git.Client, repoDir string) []publishDiagnostic {
	var diagnostics []publishDiagnostic

	for _, relPath := range registry.UniqueProjectDirs() {
		// Skip non-hidden project dirs (such as "skills") to avoid
		// flagging the canonical authoring layout used when publishing.
		if !strings.HasPrefix(relPath, ".") {
			continue
		}
		absPath := filepath.Join(repoDir, relPath)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			continue
		}

		if gitClient != nil {
			ignoreGitClient := gitClient.Copy()
			ignoreGitClient.RepoDir = repoDir
			ignored, err := ignoreGitClient.IsIgnored(context.Background(), relPath)
			if ignored {
				continue
			}
			if err != nil {
				diagnostics = append(diagnostics, publishDiagnostic{
					severity: "warning",
					message:  fmt.Sprintf("%s/ may contain installed skills that are not gitignored (could not verify: %v)", relPath, err),
				})
				continue
			}
		}

		diagnostics = append(diagnostics, publishDiagnostic{
			severity: "warning",
			message: fmt.Sprintf(
				"%s/ contains installed skills and should be added to .gitignore to avoid publishing other authors' content",
				relPath),
		})
	}

	return diagnostics
}

// semverPattern matches v-prefixed semver tags (e.g. v1.2.3).
var semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// suggestNextTag increments the patch version of a semver tag.
func suggestNextTag(latest string) string {
	m := semverPattern.FindStringSubmatch(latest)
	if m == nil {
		return ""
	}

	prefix := ""
	if strings.HasPrefix(latest, "v") {
		prefix = "v"
	}

	major, minor := m[1], m[2]
	patch := 0
	fmt.Sscanf(m[3], "%d", &patch)

	return fmt.Sprintf("%s%s.%s.%d", prefix, major, minor, patch+1)
}

// gitHubRemote holds a detected GitHub remote and its local name.
type gitHubRemote struct {
	Repo       ghrepo.Interface
	RemoteName string
}

// detectGitHubRemote attempts to detect the GitHub owner/repo from git remotes
// in the given directory. Remotes are tried in the order returned by
// gitClient.Remotes (upstream > github > origin > rest), so the first
// GitHub-pointing remote wins.
func detectGitHubRemote(gitClient *git.Client, dir string) (*gitHubRemote, error) {
	if gitClient == nil {
		return nil, nil
	}

	dirClient := gitClient.Copy()
	dirClient.RepoDir = dir

	remotes, err := dirClient.Remotes(context.Background())
	if err != nil {
		return nil, nil //nolint:nilerr // failing to list remotes is not an error; it just means no repo detected
	}
	for _, r := range remotes {
		if url, err := dirClient.RemoteURL(context.Background(), r.Name); err == nil {
			repo, parseErr := parseGitHubURL(url)
			if parseErr != nil {
				return nil, parseErr
			}
			if repo != nil {
				return &gitHubRemote{Repo: repo, RemoteName: r.Name}, nil
			}
		}
	}
	return nil, nil
}

// parseGitHubURL extracts owner/repo from a GitHub remote URL.
// Only GitHub.com URLs are recognized.
func parseGitHubURL(rawURL string) (ghrepo.Interface, error) {
	u, err := git.ParseURL(rawURL)
	if err != nil {
		return nil, nil //nolint:nilerr // unparseable URL means it's not a GitHub remote
	}
	r, err := ghrepo.FromURL(u)
	if err != nil {
		return nil, nil //nolint:nilerr // URL didn't match GitHub repo format
	}
	if err := source.ValidateSupportedHost(r.RepoHost()); err != nil {
		return nil, nil //nolint:nilerr // non-GitHub host is silently ignored
	}
	return r, nil
}

// detectMissingRepoDiagnostic explains why remote checks were skipped.
func detectMissingRepoDiagnostic(gitClient *git.Client, dir string) []publishDiagnostic {
	if gitClient == nil {
		return nil
	}

	dirGitClient := gitClient.Copy()
	dirGitClient.RepoDir = dir
	if _, err := dirGitClient.GitDir(context.Background()); err != nil {
		return []publishDiagnostic{{
			severity: "warning",
			message:  "not a git repository. Initialize with: git init && gh repo create",
		}}
	}

	remotes, err := dirGitClient.Remotes(context.Background())
	if err != nil || len(remotes) == 0 {
		return []publishDiagnostic{{
			severity: "warning",
			message:  "no git remote found. Create a GitHub repository with: gh repo create",
		}}
	}

	var urls []string
	for _, r := range remotes {
		if url, err := dirGitClient.RemoteURL(context.Background(), r.Name); err == nil {
			urls = append(urls, url)
		}
	}
	return []publishDiagnostic{{
		severity: "warning",
		message:  fmt.Sprintf("remote %q is not a GitHub repository. Skills must be hosted on GitHub for discovery", strings.Join(urls, ", ")),
	}}
}

func renderDiagnosticsTTY(opts *PublishOptions, skillCount int, diagnostics []publishDiagnostic, errors, warnings, fixes int, owner, repo string) {
	cs := opts.IO.ColorScheme()

	// Separate info messages from errors/warnings for cleaner output
	var infos, issues []publishDiagnostic
	for _, d := range diagnostics {
		if d.severity == "info" {
			infos = append(infos, d)
		} else {
			issues = append(issues, d)
		}
	}

	if len(issues) == 0 && fixes == 0 {
		fmt.Fprintf(opts.IO.Out, "%s %d skill(s) validated successfully\n", cs.SuccessIcon(), skillCount)
	} else {
		for _, d := range issues {
			var prefix string
			switch d.severity {
			case "error":
				prefix = cs.FailureIcon()
			case "warning":
				prefix = cs.WarningIcon()
			case "fixed":
				prefix = cs.SuccessIcon()
			default:
				prefix = cs.FailureIcon()
			}
			if d.skill != "" {
				fmt.Fprintf(opts.IO.Out, "%s %s: %s\n", prefix, cs.Bold(d.skill), d.message)
			} else {
				fmt.Fprintf(opts.IO.Out, "%s %s\n", prefix, d.message)
			}
		}

		fmt.Fprintln(opts.IO.Out)
		if fixes > 0 {
			fmt.Fprintf(opts.IO.Out, "Fixed %d issue(s)\n", fixes)
		}
		if errors > 0 {
			fmt.Fprintf(opts.IO.Out, "%s, %s\n",
				cs.Red(fmt.Sprintf("%d error(s)", errors)),
				cs.Yellow(fmt.Sprintf("%d warning(s)", warnings)))
		} else {
			fmt.Fprintf(opts.IO.Out, "%s\n", cs.Yellow(fmt.Sprintf("%d warning(s)", warnings)))
		}
	}

	// Always show info messages
	for _, d := range infos {
		fmt.Fprintf(opts.IO.ErrOut, "\n%s\n", d.message)
	}

	if errors == 0 && !opts.Fix {
		if owner != "" && repo != "" {
			fmt.Fprintf(opts.IO.ErrOut, "\n%s Repository: %s/%s\n", cs.Green("Ready to publish!"), owner, repo)
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "\n%s Ensure the repository has the \"agent-skills\" topic.\n", cs.Green("Ready to publish!"))
		}
	}
}

func renderDiagnosticsPlain(opts *PublishOptions, diagnostics []publishDiagnostic, errors, warnings int) {
	for _, d := range diagnostics {
		if d.severity == "info" {
			continue
		}
		fmt.Fprintf(opts.IO.Out, "%s\t%s\t%s\n", d.severity, d.skill, d.message)
	}
	if errors == 0 && warnings == 0 {
		fmt.Fprintf(opts.IO.Out, "ok\n")
	}
}

// findGitHubMetadataKeys returns metadata keys with the "github-" prefix.
func findGitHubMetadataKeys(meta map[string]interface{}) []string {
	var keys []string
	for k := range meta {
		if strings.HasPrefix(k, "github-") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// stripGitHubMetadata removes github-* keys from the metadata map and re-serializes.
func stripGitHubMetadata(content string) (string, error) {
	result, err := frontmatter.Parse(content)
	if err != nil {
		return "", err
	}

	meta, ok := result.RawYAML["metadata"].(map[string]interface{})
	if !ok {
		return content, nil
	}

	for k := range meta {
		if strings.HasPrefix(k, "github-") {
			delete(meta, k)
		}
	}

	if len(meta) == 0 {
		delete(result.RawYAML, "metadata")
	} else {
		result.RawYAML["metadata"] = meta
	}

	return frontmatter.Serialize(result.RawYAML, result.Body)
}
