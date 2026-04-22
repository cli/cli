package discovery

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
)

// specNamePattern matches the strict agentskills.io name spec:
// 1-64 chars, lowercase alphanumeric + hyphens, no leading/trailing/consecutive hyphens.
var specNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// TreeTooLargeError is returned when a repository's git tree exceeds the
// GitHub API truncation limit and full skill discovery is not possible.
type TreeTooLargeError struct {
	Owner string
	Repo  string
}

func (e *TreeTooLargeError) Error() string {
	return fmt.Sprintf("repository tree for %s/%s is too large for full discovery", e.Owner, e.Repo)
}

// safeNamePattern matches names that are safe for filesystem use during discovery.
// Allows letters (any case), numbers, hyphens, underscores, dots, and spaces.
// Must start with a letter or number. This matches copilot-agent-runtime's SKILL_NAME_REGEX.
var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\- ]*$`)

// Skill represents a discovered skill in a repository.
type Skill struct {
	Name        string
	Namespace   string // author/scope prefix for namespaced skills
	Description string
	Path        string // path within the repo, e.g. "skills/git-commit"
	BlobSHA     string // SHA of the SKILL.md blob
	TreeSHA     string // SHA of the skill directory tree
	Convention  string // which directory convention matched
}

// DisplayName returns the skill name, prefixed with namespace if present
// to disambiguate skills from different authors in the same repository.
// Skills discovered via non-standard conventions (plugins, root) include
// a convention tag to distinguish them from identically-named skills in
// the standard skills/ directory.
func (s Skill) DisplayName() string {
	name := s.Name
	if s.Namespace != "" {
		name = s.Namespace + "/" + name
	}
	switch s.Convention {
	case "plugins":
		return "[plugins] " + name
	case "root":
		return "[root] " + name
	case "hidden-dir", "hidden-dir-namespaced":
		return "[hidden-dir] " + name
	default:
		return name
	}
}

// InstallName returns the relative path used for the install directory.
// For namespaced skills it returns "namespace/name" (creating a nested directory),
// otherwise it returns the plain name. Callers should use filepath.FromSlash
// when building OS-specific paths from this value.
func (s Skill) InstallName() string {
	if s.Namespace != "" {
		return s.Namespace + "/" + s.Name
	}
	return s.Name
}

// IsHiddenDirConvention returns true if the skill was discovered in a hidden
// (dot-prefixed) directory such as .claude/skills/ or .agents/skills/.
func (s Skill) IsHiddenDirConvention() bool {
	return s.Convention == "hidden-dir" || s.Convention == "hidden-dir-namespaced"
}

// HasHiddenDirSkills returns true if any of the given skills were discovered
// in hidden directories.
func HasHiddenDirSkills(skills []Skill) bool {
	for _, s := range skills {
		if s.IsHiddenDirConvention() {
			return true
		}
	}
	return false
}

// ResolvedRef contains the resolved git reference and its SHA.
type ResolvedRef struct {
	Ref string // fully qualified ref (refs/heads/*, refs/tags/*) or commit SHA
	SHA string // commit SHA
}

// IsFullyQualifiedRef returns true if ref uses the "refs/heads/" or "refs/tags/" prefix.
func IsFullyQualifiedRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/tags/")
}

// ShortRef strips the "refs/heads/" or "refs/tags/" prefix from a fully qualified ref,
// returning the short name. If the ref is not fully qualified it is returned as-is.
func ShortRef(ref string) string {
	if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return after
	}
	if after, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
		return after
	}
	return ref
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int    `json:"size"`
}

// SkillFile represents a file within a skill directory.
type SkillFile struct {
	Path string // relative path within the skill directory
	SHA  string // blob SHA for fetching content
	Size int    // file size in bytes
}

type treeResponse struct {
	SHA       string      `json:"sha"`
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

type RepoVisibility string

const (
	RepoVisibilityPublic   RepoVisibility = "public"
	RepoVisibilityPrivate  RepoVisibility = "private"
	RepoVisibilityInternal RepoVisibility = "internal"
)

func parseRepoVisibility(s string) (RepoVisibility, error) {
	switch s {
	case "public":
		return RepoVisibilityPublic, nil
	case "private":
		return RepoVisibilityPrivate, nil
	case "internal":
		return RepoVisibilityInternal, nil
	default:
		return "", fmt.Errorf("unknown repository visibility: %q", s)
	}
}

// FetchRepoVisibility returns the repository visibility: "public", "private", or "internal".
func FetchRepoVisibility(client *api.Client, host, owner, repo string) (RepoVisibility, error) {
	apiPath := fmt.Sprintf("repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	var resp struct {
		Visibility string `json:"visibility"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return "", err
	}
	return parseRepoVisibility(resp.Visibility)
}

// ResolveRef determines the git ref to use for a given owner/repo.
// Priority: explicit version > latest release tag > default branch.
func ResolveRef(client *api.Client, host, owner, repo, version string) (*ResolvedRef, error) {
	if version != "" {
		return resolveExplicitRef(client, host, owner, repo, version)
	}
	ref, err := resolveLatestRelease(client, host, owner, repo)
	if err == nil {
		return ref, nil
	}
	// Only fall back to the default branch when the repository genuinely
	// has no releases (404) or the latest release has no tag. Any other
	// API error (403, 500, network failure, …) is surfaced immediately
	// so it cannot silently mask problems and cause an unexpected ref to
	// be used.
	var nre *noReleasesError
	if !errors.As(err, &nre) {
		return nil, err
	}
	return resolveDefaultBranch(client, host, owner, repo)
}

// resolveExplicitRef resolves a user-supplied version string. It supports:
//   - fully qualified refs: "refs/tags/v1.0" or "refs/heads/main"
//   - short names: tried as branch first, then tag, then commit SHA
//   - bare SHAs: resolved as commit SHA
//
// When a short name matches both a branch and a tag, the branch wins.
// The returned Ref is always a fully qualified ref (refs/heads/* or refs/tags/*)
// unless the input resolves to a bare commit SHA.
func resolveExplicitRef(client *api.Client, host, owner, repo, ref string) (*ResolvedRef, error) {
	// Handle fully-qualified refs: resolve directly without ambiguity.
	if after, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
		return resolveTagRef(client, host, owner, repo, after)
	}
	if after, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return resolveBranchRef(client, host, owner, repo, after)
	}

	// Short name: try branch first, then tag, then commit SHA.
	// Only fall through on 404 (not found); surface other errors
	// (403, 500, network) immediately to avoid masking real failures.
	if resolved, err := resolveBranchRef(client, host, owner, repo, ref); err == nil {
		return resolved, nil
	} else if !isNotFound(err) {
		return nil, err
	}
	if resolved, err := resolveTagRef(client, host, owner, repo, ref); err == nil {
		return resolved, nil
	} else if !isNotFound(err) {
		return nil, err
	}

	commitPath := fmt.Sprintf("repos/%s/%s/commits/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))
	var commitResp struct {
		SHA string `json:"sha"`
	}
	if err := client.REST(host, "GET", commitPath, nil, &commitResp); err == nil {
		return &ResolvedRef{Ref: commitResp.SHA, SHA: commitResp.SHA}, nil
	} else if !isNotFound(err) {
		return nil, err
	}

	return nil, fmt.Errorf("ref %q not found as branch, tag, or commit in %s/%s", ref, owner, repo)
}

// resolveTagRef looks up a tag by short name and returns a fully qualified ref.
// For annotated tags, the tag object is dereferenced to obtain the commit SHA.
func resolveTagRef(client *api.Client, host, owner, repo, tag string) (*ResolvedRef, error) {
	tagPath := fmt.Sprintf("repos/%s/%s/git/ref/tags/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag))
	var refResp struct {
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := client.REST(host, "GET", tagPath, nil, &refResp); err != nil {
		return nil, fmt.Errorf("tag %q not found in %s/%s: %w", tag, owner, repo, err)
	}
	sha := refResp.Object.SHA
	if refResp.Object.Type == "tag" {
		derefPath := fmt.Sprintf("repos/%s/%s/git/tags/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))
		var tagResp struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		if err := client.REST(host, "GET", derefPath, nil, &tagResp); err != nil {
			return nil, fmt.Errorf("could not dereference annotated tag %q: %w", tag, err)
		}
		sha = tagResp.Object.SHA
	}
	return &ResolvedRef{Ref: "refs/tags/" + tag, SHA: sha}, nil
}

// resolveBranchRef looks up a branch by short name and returns a fully qualified ref.
func resolveBranchRef(client *api.Client, host, owner, repo, branch string) (*ResolvedRef, error) {
	refPath := fmt.Sprintf("repos/%s/%s/git/ref/heads/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(branch))
	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := client.REST(host, "GET", refPath, nil, &refResp); err != nil {
		return nil, fmt.Errorf("branch %q not found in %s/%s: %w", branch, owner, repo, err)
	}
	return &ResolvedRef{Ref: "refs/heads/" + branch, SHA: refResp.Object.SHA}, nil
}

// isNotFound returns true if the error is an HTTP 404 response.
func isNotFound(err error) bool {
	var httpErr api.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// noReleasesError signals that the repository has no usable releases,
// which is the only case where ResolveRef should fall back to the
// default branch.
type noReleasesError struct {
	reason string
}

func (e *noReleasesError) Error() string { return e.reason }

func resolveLatestRelease(client *api.Client, host, owner, repo string) (*ResolvedRef, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo))
	var resp struct {
		TagName string `json:"tag_name"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		// A 404 means the repository has no releases. This is the
		// only case where falling back to the default branch is safe.
		// Any other HTTP error (403, 500, …) or network failure is
		// returned as-is so ResolveRef surfaces it rather than
		// silently falling back.
		if isNotFound(err) {
			return nil, &noReleasesError{reason: fmt.Sprintf("no releases found for %s/%s", owner, repo)}
		}
		return nil, fmt.Errorf("could not fetch latest release: %w", err)
	}
	if resp.TagName == "" {
		return nil, &noReleasesError{reason: "latest release has no tag"}
	}
	return resolveTagRef(client, host, owner, repo, resp.TagName)
}

func resolveDefaultBranch(client *api.Client, host, owner, repo string) (*ResolvedRef, error) {
	apiPath := fmt.Sprintf("repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
	var resp struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return nil, fmt.Errorf("could not determine default branch: %w", err)
	}
	branch := resp.DefaultBranch
	if branch == "" {
		return nil, fmt.Errorf("could not determine default branch for %s/%s", owner, repo)
	}
	return resolveBranchRef(client, host, owner, repo, branch)
}

// skillMatch represents a matched SKILL.md file and its convention.
type skillMatch struct {
	entry      treeEntry
	name       string
	namespace  string
	skillDir   string
	convention string
}

// MatchesSkillPath checks if a file path matches any known skill convention
// and returns the skill name. Returns empty string if the path doesn't match.
func MatchesSkillPath(filePath string) string {
	m := matchSkillConventions(treeEntry{Path: filePath})
	if m == nil {
		return ""
	}
	return m.name
}

// MatchSkillPath checks if a file path matches any known skill convention
// and returns the skill name and namespace. Returns empty strings if the
// path doesn't match. The namespace is non-empty for namespaced skills
// (e.g. skills/author/name/SKILL.md) and plugin skills.
func MatchSkillPath(filePath string) (name, namespace string) {
	m := matchSkillConventions(treeEntry{Path: filePath})
	if m == nil {
		return "", ""
	}
	return m.name, m.namespace
}

// matchSkillConventions checks if a blob path matches any known skill convention.
func matchSkillConventions(entry treeEntry) *skillMatch {
	if path.Base(entry.Path) != "SKILL.md" {
		return nil
	}

	dir := path.Dir(entry.Path)
	parentDir := path.Dir(dir)
	skillName := path.Base(dir)

	if !validateName(skillName) {
		return nil
	}

	if parentDir == "skills" {
		return &skillMatch{entry: entry, name: skillName, skillDir: dir, convention: "skills"}
	}

	grandparentDir := path.Dir(parentDir)
	if grandparentDir == "skills" {
		namespace := path.Base(parentDir)
		if !validateName(namespace) {
			return nil
		}
		return &skillMatch{entry: entry, name: skillName, namespace: namespace, skillDir: dir, convention: "skills-namespaced"}
	}

	if path.Base(parentDir) == "skills" && path.Dir(grandparentDir) == "plugins" {
		namespace := path.Base(grandparentDir)
		if !validateName(namespace) {
			return nil
		}
		return &skillMatch{entry: entry, name: skillName, namespace: namespace, skillDir: dir, convention: "plugins"}
	}

	// Deeply nested skills/ directory: <prefix>/skills/<name>/SKILL.md
	// Matches skills/ at any depth, not just at the repository root.
	// Exclude paths with dot-prefixed segments (handled by
	// matchHiddenDirConventions) and paths under a plugins/ directory
	// (handled by the plugins convention above).
	if path.Base(parentDir) == "skills" && !hasHiddenSegment(entry.Path) && !hasPluginsAncestor(entry.Path) {
		return &skillMatch{entry: entry, name: skillName, skillDir: dir, convention: "skills"}
	}

	// Deeply nested namespaced: <prefix>/skills/<namespace>/<name>/SKILL.md
	if path.Base(grandparentDir) == "skills" && !hasHiddenSegment(entry.Path) && !hasPluginsAncestor(entry.Path) {
		namespace := path.Base(parentDir)
		if !validateName(namespace) {
			return nil
		}
		return &skillMatch{entry: entry, name: skillName, namespace: namespace, skillDir: dir, convention: "skills-namespaced"}
	}

	if parentDir == "." && skillName != "skills" && skillName != "plugins" && !strings.HasPrefix(skillName, ".") {
		return &skillMatch{entry: entry, name: skillName, skillDir: dir, convention: "root"}
	}

	return nil
}

// matchHiddenDirConventions checks if a blob path matches a skill convention
// under a hidden (dot-prefixed) root directory. These patterns mirror the
// standard skills/ conventions but rooted under .{host}/skills/:
//
//   - .{host}/skills/*/SKILL.md         -> "hidden-dir"
//   - .{host}/skills/{scope}/*/SKILL.md -> "hidden-dir-namespaced"
func matchHiddenDirConventions(entry treeEntry) *skillMatch {
	if path.Base(entry.Path) != "SKILL.md" {
		return nil
	}

	// .{host}/skills/*
	// .{host}/skills/{scope}/*
	dir := path.Dir(entry.Path)
	skillName := path.Base(dir)

	if !validateName(skillName) {
		return nil
	}

	// .{host}/skills
	// .{host}/skills/{scope}
	parentDir := path.Dir(dir)

	// .{host}/skills/*/SKILL.md
	if path.Base(parentDir) == "skills" {
		hiddenRoot := path.Dir(parentDir)
		if path.Dir(hiddenRoot) == "." && strings.HasPrefix(hiddenRoot, ".") {
			return &skillMatch{entry: entry, name: skillName, skillDir: dir, convention: "hidden-dir"}
		}
	}

	// .{host}/skills/{scope}/*/SKILL.md
	grandparentDir := path.Dir(parentDir)
	if path.Base(grandparentDir) == "skills" {
		hiddenRoot := path.Dir(grandparentDir)
		if path.Dir(hiddenRoot) == "." && strings.HasPrefix(hiddenRoot, ".") {
			namespace := path.Base(parentDir)
			if !validateName(namespace) {
				return nil
			}
			return &skillMatch{entry: entry, name: skillName, namespace: namespace, skillDir: dir, convention: "hidden-dir-namespaced"}
		}
	}

	return nil
}

// DiscoverOptions controls optional discovery behaviors.
type DiscoverOptions struct {
}

// DiscoverSkills finds all non-hidden-dir skills in a repository at the given
// commit SHA. Hidden-dir skills are excluded; use DiscoverSkillsWithOptions to
// retrieve all skills including those in hidden directories.
func DiscoverSkills(client *api.Client, host, owner, repo, commitSHA string) ([]Skill, error) {
	all, err := DiscoverSkillsWithOptions(client, host, owner, repo, commitSHA, DiscoverOptions{})
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, s := range all {
		if !s.IsHiddenDirConvention() {
			skills = append(skills, s)
		}
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf(
			"no skills found in %s/%s\n"+
				"  Expected skills in skills/*/SKILL.md, skills/{scope}/*/SKILL.md,\n"+
				"  */SKILL.md, or plugins/*/skills/*/SKILL.md\n"+
				"  This repository may be a curated list rather than a skills publisher",
			owner, repo,
		)
	}
	return skills, nil
}

// DiscoverSkillsWithOptions finds all skills in a repository at the given
// commit SHA, with configurable discovery behavior.
func DiscoverSkillsWithOptions(client *api.Client, host, owner, repo, commitSHA string, opts DiscoverOptions) ([]Skill, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/git/trees/%s?recursive=true", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(commitSHA))
	var tree treeResponse
	if err := client.REST(host, "GET", apiPath, nil, &tree); err != nil {
		return nil, fmt.Errorf("could not fetch repository tree: %w", err)
	}

	if tree.Truncated {
		return nil, &TreeTooLargeError{Owner: owner, Repo: repo}
	}

	treeSHAs := make(map[string]string)
	for _, entry := range tree.Tree {
		if entry.Type == "tree" {
			treeSHAs[entry.Path] = entry.SHA
		}
	}

	seen := make(map[string]bool)
	var matches []skillMatch
	for _, entry := range tree.Tree {
		if entry.Type != "blob" {
			continue
		}
		m := matchSkillConventions(entry)
		if m == nil {
			m = matchHiddenDirConventions(entry)
		}
		if m == nil {
			continue
		}
		if seen[m.skillDir] {
			continue
		}
		seen[m.skillDir] = true
		matches = append(matches, *m)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf(
			"no skills found in %s/%s\n"+
				"  Expected skills in skills/*/SKILL.md, skills/{scope}/*/SKILL.md,\n"+
				"  {prefix}/skills/*/SKILL.md, {prefix}/skills/{scope}/*/SKILL.md,\n"+
				"  */SKILL.md, or plugins/*/skills/*/SKILL.md\n"+
				"  This repository may be a curated list rather than a skills publisher",
			owner, repo,
		)
	}

	var skills []Skill
	for _, m := range matches {
		skills = append(skills, Skill{
			Name:       m.name,
			Namespace:  m.namespace,
			Path:       m.skillDir,
			BlobSHA:    m.entry.SHA,
			TreeSHA:    treeSHAs[m.skillDir],
			Convention: m.convention,
		})
	}

	sort.SliceStable(skills, func(i, j int) bool {
		return skills[i].DisplayName() < skills[j].DisplayName()
	})

	return skills, nil
}

// fetchDescription fetches and parses the frontmatter description for a skill.
func fetchDescription(client *api.Client, host, owner, repo string, skill *Skill) string {
	if skill.BlobSHA == "" {
		return ""
	}
	content, err := FetchBlob(client, host, owner, repo, skill.BlobSHA)
	if err != nil {
		return ""
	}
	result, err := frontmatter.Parse(content)
	if err != nil {
		return ""
	}
	return result.Metadata.Description
}

// FetchDescriptionsConcurrent fetches descriptions with bounded concurrency.
func FetchDescriptionsConcurrent(client *api.Client, host, owner, repo string, skills []Skill, onProgress func(done, total int)) {
	total := 0
	for _, s := range skills {
		if s.Description == "" {
			total++
		}
	}
	if total == 0 {
		return
	}

	const maxWorkers = 10
	var wg sync.WaitGroup
	var done atomic.Int32

	jobs := make(chan *Skill)

	workers := min(maxWorkers, total)
	for range workers {
		wg.Go(func() {
			for s := range jobs {
				s.Description = fetchDescription(client, host, owner, repo, s)

				d := int(done.Add(1))
				if onProgress != nil {
					onProgress(d, total)
				}
			}
		})
	}

	for i := range skills {
		if skills[i].Description == "" {
			jobs <- &skills[i]
		}
	}
	close(jobs)
	wg.Wait()
}

// DiscoverSkillByPath looks up a single skill by its exact path in the repository.
func DiscoverSkillByPath(client *api.Client, host, owner, repo, commitSHA, skillPath string) (*Skill, error) {
	skillPath = strings.TrimSuffix(skillPath, "/SKILL.md")
	skillPath = strings.TrimSuffix(skillPath, "/")

	skillName := path.Base(skillPath)
	if !validateName(skillName) {
		return nil, fmt.Errorf("invalid skill name %q", skillName)
	}

	parentPath := path.Dir(skillPath)
	apiPath := fmt.Sprintf("repos/%s/%s/contents/%s?ref=%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(parentPath), commitSHA)

	var contents []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		SHA  string `json:"sha"`
		Type string `json:"type"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &contents); err != nil {
		return nil, fmt.Errorf("path %q not found in %s/%s: %w", parentPath, owner, repo, err)
	}

	var treeSHA string
	for _, entry := range contents {
		if entry.Name == skillName && entry.Type == "dir" {
			treeSHA = entry.SHA
			break
		}
	}
	if treeSHA == "" {
		return nil, fmt.Errorf("skill directory %q not found in %s/%s", skillPath, owner, repo)
	}

	skillTreePath := fmt.Sprintf("repos/%s/%s/git/trees/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(treeSHA))
	var skillTree treeResponse
	if err := client.REST(host, "GET", skillTreePath, nil, &skillTree); err != nil {
		return nil, fmt.Errorf("could not read skill directory: %w", err)
	}

	var blobSHA string
	for _, entry := range skillTree.Tree {
		if entry.Path == "SKILL.md" && entry.Type == "blob" {
			blobSHA = entry.SHA
			break
		}
	}
	if blobSHA == "" {
		return nil, fmt.Errorf("no SKILL.md found in %s", skillPath)
	}

	var namespace, convention string
	parts := strings.Split(skillPath, "/")
	for i, p := range parts {
		if p != "skills" {
			continue
		}

		// Plugin convention: .../plugins/<ns>/skills/<name>
		if i >= 2 && parts[i-2] == "plugins" {
			namespace = parts[i-1]
			convention = "plugins"
			break
		}

		// Namespaced skill convention: .../skills/<ns>/<name>
		afterSkills := parts[i+1:]
		if len(afterSkills) >= 2 {
			namespace = afterSkills[0]
		}
		break
	}

	skill := &Skill{
		Name:       skillName,
		Namespace:  namespace,
		Convention: convention,
		Path:       skillPath,
		BlobSHA:    blobSHA,
		TreeSHA:    treeSHA,
	}

	skill.Description = fetchDescription(client, host, owner, repo, skill)

	return skill, nil
}

// DiscoverSkillFiles returns all file paths belonging to a skill directory
// by fetching the skill's subtree directly using its tree SHA.
func DiscoverSkillFiles(client *api.Client, host, owner, repo, treeSHA, skillPath string) ([]SkillFile, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/git/trees/%s?recursive=true", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(treeSHA))
	var tree treeResponse
	if err := client.REST(host, "GET", apiPath, nil, &tree); err != nil {
		return nil, fmt.Errorf("could not fetch skill tree: %w", err)
	}

	if tree.Truncated {
		// Recursive fetch was truncated. Fall back to walking subtrees individually.
		return walkTree(client, host, owner, repo, treeSHA, skillPath, 0)
	}

	var files []SkillFile
	for _, entry := range tree.Tree {
		if entry.Type == "blob" {
			files = append(files, SkillFile{
				Path: skillPath + "/" + entry.Path,
				SHA:  entry.SHA,
				Size: entry.Size,
			})
		}
	}

	return files, nil
}

// ListSkillFiles returns all files in a skill directory as public SkillFile
// structs with paths relative to the skill root.
func ListSkillFiles(client *api.Client, host, owner, repo, treeSHA string) ([]SkillFile, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/git/trees/%s?recursive=true", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(treeSHA))
	var tree treeResponse
	if err := client.REST(host, "GET", apiPath, nil, &tree); err != nil {
		return nil, fmt.Errorf("could not fetch skill tree: %w", err)
	}

	if tree.Truncated {
		// Fall back to non-recursive traversal when the tree is too large.
		return walkTree(client, host, owner, repo, treeSHA, "", 0)
	}

	var files []SkillFile
	for _, entry := range tree.Tree {
		if entry.Type == "blob" {
			files = append(files, SkillFile{
				Path: entry.Path,
				SHA:  entry.SHA,
				Size: entry.Size,
			})
		}
	}
	return files, nil
}

// maxTreeDepth bounds the recursion in walkTree to prevent unbounded
// API calls on deeply nested repositories.
const maxTreeDepth = 20

// walkTree enumerates files by fetching each tree level individually,
// avoiding the truncation limit of the recursive tree API. Recursion
// depth is bounded by maxTreeDepth to prevent unbounded API calls.
func walkTree(client *api.Client, host, owner, repo, sha, prefix string, depth int) ([]SkillFile, error) {
	if depth > maxTreeDepth {
		return nil, fmt.Errorf("tree depth exceeds %d levels at %s", maxTreeDepth, prefix)
	}
	apiPath := fmt.Sprintf("repos/%s/%s/git/trees/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))
	var tree treeResponse
	if err := client.REST(host, "GET", apiPath, nil, &tree); err != nil {
		return nil, fmt.Errorf("could not fetch tree %s: %w", prefix, err)
	}

	var files []SkillFile
	for _, entry := range tree.Tree {
		entryPath := entry.Path
		if prefix != "" {
			entryPath = prefix + "/" + entry.Path
		}
		switch entry.Type {
		case "blob":
			files = append(files, SkillFile{Path: entryPath, SHA: entry.SHA, Size: entry.Size})
		case "tree":
			sub, err := walkTree(client, host, owner, repo, entry.SHA, entryPath, depth+1)
			if err != nil {
				return nil, err
			}
			files = append(files, sub...)
		}
	}
	return files, nil
}

// FetchBlob retrieves the content of a blob by SHA.
func FetchBlob(client *api.Client, host, owner, repo, sha string) (string, error) {
	apiPath := fmt.Sprintf("repos/%s/%s/git/blobs/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(sha))
	var resp struct {
		SHA      string `json:"sha"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := client.REST(host, "GET", apiPath, nil, &resp); err != nil {
		return "", fmt.Errorf("could not fetch blob: %w", err)
	}

	if resp.Encoding != "base64" {
		return "", fmt.Errorf("unexpected blob encoding: %s", resp.Encoding)
	}

	// GitHub API returns base64 with embedded newlines; use the StdEncoding
	// decoder via a reader to handle them transparently.
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, strings.NewReader(resp.Content)))
	if err != nil {
		return "", fmt.Errorf("could not decode blob content: %w", err)
	}

	return string(decoded), nil
}

// DiscoverLocalSkills finds non-hidden-dir skills in a local directory using
// the same conventions as remote discovery. Hidden-dir skills are excluded; use
// DiscoverLocalSkillsWithOptions to retrieve all skills including those in
// hidden directories.
func DiscoverLocalSkills(dir string) ([]Skill, error) {
	all, err := DiscoverLocalSkillsWithOptions(dir, DiscoverOptions{})
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, s := range all {
		if !s.IsHiddenDirConvention() {
			skills = append(skills, s)
		}
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf(
			"no skills found in %s\n"+
				"  Expected SKILL.md in the directory, or skills in skills/*/SKILL.md,\n"+
				"  skills/{scope}/*/SKILL.md, */SKILL.md, or plugins/*/skills/*/SKILL.md",
			dir,
		)
	}
	return skills, nil
}

// DiscoverLocalSkillsWithOptions finds skills in a local directory using the
// same conventions as remote discovery, with configurable discovery behavior.
func DiscoverLocalSkillsWithOptions(dir string, opts DiscoverOptions) ([]Skill, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("could not resolve path: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("could not access %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	if _, err := os.Stat(filepath.Join(absDir, "SKILL.md")); err == nil {
		skill, err := localSkillFromDir(absDir)
		if err != nil {
			return nil, err
		}
		skill.Path = "."
		return []Skill{*skill}, nil
	}

	var skills []Skill
	seen := make(map[string]bool)

	err = filepath.Walk(absDir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip symlinks to avoid following links outside the source tree.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() || info.Name() != "SKILL.md" {
			return nil
		}

		relPath, relErr := filepath.Rel(absDir, p)
		if relErr != nil {
			return relErr
		}
		relPath = filepath.ToSlash(relPath)

		entry := treeEntry{Path: relPath, Type: "blob"}
		m := matchSkillConventions(entry)
		if m == nil {
			m = matchHiddenDirConventions(entry)
		}
		if m == nil {
			return nil
		}
		if seen[m.skillDir] {
			return nil
		}
		seen[m.skillDir] = true

		skill, skillErr := localSkillFromDir(filepath.Join(absDir, filepath.FromSlash(m.skillDir)))
		if skillErr != nil {
			return nil //nolint:nilerr // intentionally skip files that aren't valid skills
		}
		skill.Path = m.skillDir
		skill.Namespace = m.namespace
		skill.Convention = m.convention
		skills = append(skills, *skill)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not walk directory: %w", err)
	}

	if len(skills) == 0 {
		return nil, fmt.Errorf(
			"no skills found in %s\n"+
				"  Expected SKILL.md in the directory, or skills in skills/*/SKILL.md,\n"+
				"  skills/{scope}/*/SKILL.md, {prefix}/skills/*/SKILL.md,\n"+
				"  {prefix}/skills/{scope}/*/SKILL.md, */SKILL.md, or\n"+
				"  plugins/*/skills/*/SKILL.md",
			dir,
		)
	}

	return skills, nil
}

func localSkillFromDir(dir string) (*Skill, error) {
	skillFile := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", skillFile, err)
	}

	name := filepath.Base(dir)
	var description string

	result, parseErr := frontmatter.Parse(string(data))
	if parseErr == nil {
		if result.Metadata.Name != "" {
			name = result.Metadata.Name
		}
		description = result.Metadata.Description
	}

	if !validateName(name) {
		return nil, fmt.Errorf("invalid skill name %q in %s", name, dir)
	}

	return &Skill{
		Name:        name,
		Description: description,
		Path:        filepath.Base(dir),
	}, nil
}

// validateName checks if a skill name is safe for use (filesystem-safe).
func validateName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	return safeNamePattern.MatchString(name)
}

// hasHiddenSegment reports whether any path component starts with a dot.
func hasHiddenSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// hasPluginsAncestor reports whether any path component is "plugins".
func hasPluginsAncestor(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "plugins" {
			return true
		}
	}
	return false
}

// IsSpecCompliant checks if a skill name matches the strict agentskills.io spec.
func IsSpecCompliant(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	if strings.Contains(name, "--") {
		return false
	}
	return specNamePattern.MatchString(name)
}
