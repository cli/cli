package search

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/frontmatter"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/internal/skills/source"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 15
	maxResults   = 1000 // GitHub Code Search API hard limit

	// searchPageSize is the number of raw results to request from the
	// GitHub Search API per call (max allowed).
	searchPageSize = 100
)

// SkillSearchFields defines the set of fields available for --json output.
var SkillSearchFields = []string{
	"repo",
	"skillName",
	"namespace",
	"description",
	"stars",
	"path",
}

type SearchOptions struct {
	IO             *iostreams.IOStreams
	Telemetry      ghtelemetry.EventRecorder
	HttpClient     func() (*http.Client, error)
	Config         func() (gh.Config, error)
	Prompter       prompter.Prompter
	ExecutablePath string // path to the current gh binary for install subprocess
	Exporter       cmdutil.Exporter

	// User inputs
	Query string
	Owner string // optional: scope results to a specific GitHub owner
	Page  int
	Limit int
}

// NewCmdSearch creates the "skills search" command.
func NewCmdSearch(f *cmdutil.Factory, telemetry ghtelemetry.CommandRecorder, runF func(*SearchOptions) error) *cobra.Command {
	opts := &SearchOptions{
		IO:             f.IOStreams,
		Telemetry:      telemetry,
		HttpClient:     f.HttpClient,
		Config:         f.Config,
		Prompter:       f.Prompter,
		ExecutablePath: f.ExecutablePath,
	}

	cmd := &cobra.Command{
		Use:   "search <query> [flags]",
		Short: "Search for skills across GitHub (preview)",
		Long: heredoc.Docf(`
			Search across all public GitHub repositories for skills matching a keyword.

			Uses the GitHub Code Search API to find %[1]sSKILL.md%[1]s files whose name or
			description matches the query term.

			Results are ranked by relevance: skills whose name contains the query
			term appear first.

			Use %[1]s--owner%[1]s to scope results to a specific GitHub user or organization.

			In interactive mode, you can select skills from the results to install directly.
		`, "`"),
		Example: heredoc.Doc(`
			# Search for skills related to terraform
			$ gh skill search terraform

			# Search for skills from a specific owner
			$ gh skill search terraform --owner hashicorp

			# View the second page of results
			$ gh skill search terraform --page 2

			# Limit results to 5
			$ gh skill search terraform --limit 5
		`),
		Args: cmdutil.MinimumArgs(1, "cannot search: query argument required"),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.Join(args, " ")

			if len(strings.TrimSpace(opts.Query)) < 2 {
				return cmdutil.FlagErrorf("search query must be at least 2 characters")
			}

			if opts.Page < 1 {
				return cmdutil.FlagErrorf("invalid page number: %d", opts.Page)
			}

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %d", opts.Limit)
			}

			opts.Owner = strings.TrimSpace(opts.Owner)
			if opts.Owner != "" && !couldBeOwner(opts.Owner) {
				return cmdutil.FlagErrorf("invalid owner %q: must be a valid GitHub username or organization", opts.Owner)
			}

			if runF != nil {
				return runF(opts)
			}
			return searchRun(opts)
		},
	}

	cmd.Flags().IntVar(&opts.Page, "page", 1, "Page number of results to fetch")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of results per page")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "Filter results to a specific GitHub user or organization")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, SkillSearchFields)

	return cmd
}

// codeSearchResult represents the GitHub Code Search API response.
type codeSearchResult struct {
	TotalCount        int              `json:"total_count"`
	IncompleteResults bool             `json:"incomplete_results"`
	Items             []codeSearchItem `json:"items"`
}

// codeSearchItem represents a single code search hit.
type codeSearchItem struct {
	Name       string               `json:"name"`
	Path       string               `json:"path"`
	SHA        string               `json:"sha"`
	Repository codeSearchRepository `json:"repository"`
}

// codeSearchRepository is the repo info embedded in a code search hit.
type codeSearchRepository struct {
	FullName string `json:"full_name"`
}

// skillResult is a deduplicated search result.
type skillResult struct {
	Repo        string
	Owner       string // parsed from Repo
	RepoName    string // parsed from Repo
	SkillName   string
	Namespace   string // namespace prefix: author/scope for skills/{author}/* or plugin name for plugins/{plugin}/skills/*
	Description string
	Path        string // original file path (e.g. skills/terraform/SKILL.md)
	BlobSHA     string
	Stars       int // repository stargazer count
}

// qualifiedName returns the namespace-qualified skill name (e.g. "author/skill")
// or just the skill name if there is no namespace.
func (s skillResult) qualifiedName() string {
	if s.Namespace != "" {
		return s.Namespace + "/" + s.SkillName
	}
	return s.SkillName
}

// ExportData implements cmdutil.exportable for --json output.
func (s skillResult) ExportData(fields []string) map[string]any {
	data := map[string]any{}
	for _, f := range fields {
		switch f {
		case "repo":
			data[f] = s.Repo
		case "skillName":
			data[f] = s.SkillName
		case "namespace":
			data[f] = s.Namespace
		case "description":
			data[f] = s.Description
		case "stars":
			data[f] = s.Stars
		case "path":
			data[f] = s.Path
		}
	}
	return data
}

func searchRun(opts *SearchOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()
	if err := source.ValidateSupportedHost(host); err != nil {
		return err
	}

	opts.IO.StartProgressIndicatorWithLabel("Searching for skills")

	skills, err := searchByKeyword(apiClient, host, opts.Query, opts.Owner, opts.Page, opts.Limit)
	if err != nil {
		opts.IO.StopProgressIndicator()
		return err
	}

	if len(skills) == 0 {
		opts.IO.StopProgressIndicator()
		return noResults(opts, noResultsMessage(opts))
	}

	// Pre-rank before expensive enrichment, then truncate working set.
	rankByRelevance(skills, opts.Query)
	skills = truncateForProcessing(skills, opts.Page, opts.Limit)

	enrichSkills(apiClient, host, skills)
	opts.IO.StopProgressIndicator()

	// Filter out noise and re-rank with enriched data (descriptions, stars).
	skills = filterByRelevance(skills, opts.Query)
	if len(skills) == 0 {
		return noResults(opts, noResultsMessage(opts))
	}
	rankByRelevance(skills, opts.Query)

	// Collapse duplicate skill names across repos, keeping up to 3
	// top-ranked instances of each. Prevents aggregator repos
	// (which copy popular skills) from flooding results.
	skills = deduplicateByName(skills)

	// Paginate to the requested page window.
	var totalPages int
	skills, totalPages = paginate(skills, opts.Page, opts.Limit)
	if len(skills) == 0 {
		msg := fmt.Sprintf("no skills found on page %d for query %q", opts.Page, opts.Query)
		if opts.Owner != "" {
			msg = fmt.Sprintf("no skills found on page %d for query %q from owner %q", opts.Page, opts.Query, opts.Owner)
		}
		return noResults(opts, msg)
	}

	return renderResults(opts, skills, totalPages)
}

// noResultsMessage returns an appropriate "no results" message.
func noResultsMessage(opts *SearchOptions) string {
	if opts.Owner != "" {
		return fmt.Sprintf("no skills found matching %q from owner %q", opts.Query, opts.Owner)
	}
	return fmt.Sprintf("no skills found matching %q", opts.Query)
}

// searchByKeyword runs parallel searches: content match, path match, owner
// match (for single-word queries), and (for multi-word queries) a hyphenated
// content match to catch skill names like "mcp-apps" when the user types
// "mcp apps". When owner is non-empty, all queries are scoped to that
// GitHub user/org via user:<owner> and the implicit owner search is skipped.
func searchByKeyword(client *api.Client, host, queryTerm, owner string, page, limit int) ([]skillResult, error) {
	ownerScope := ""
	if owner != "" {
		ownerScope = " user:" + owner
	}

	primaryQ := fmt.Sprintf("filename:SKILL.md %s%s", queryTerm, ownerScope)
	pathTerm := strings.ReplaceAll(queryTerm, " ", "-")
	pathQ := fmt.Sprintf("filename:SKILL.md path:%s%s", pathTerm, ownerScope)

	var (
		primaryItems []codeSearchItem
		primaryErr   error
		pathResult   *codeSearchResult
		pathErr      error
		ownerResult  *codeSearchResult
		ownerErr     error
		hyphenResult *codeSearchResult
		hyphenErr    error
	)

	hasSpaces := strings.Contains(queryTerm, " ")

	var wg sync.WaitGroup

	wg.Go(func() {
		pathResult, pathErr = executeSearch(client, host, pathQ, 1, searchPageSize)
	})

	// When no explicit --owner is set and the query looks like it could be a
	// GitHub username, fire an additional user:<query> search to discover
	// skills published by that org. Results compete on the same footing as
	// everything else (no scoring boost).
	if owner == "" && couldBeOwner(queryTerm) {
		ownerQ := fmt.Sprintf("filename:SKILL.md user:%s", queryTerm)
		wg.Go(func() {
			ownerResult, ownerErr = executeSearch(client, host, ownerQ, 1, searchPageSize)
		})
	}

	// When the query has spaces (e.g. "mcp apps"), run an additional content
	// search with the hyphenated form ("mcp-apps") so we don't miss skills
	// whose names use hyphens as word separators.
	if hasSpaces {
		hyphenQ := fmt.Sprintf("filename:SKILL.md %s%s", pathTerm, ownerScope)
		wg.Go(func() {
			hyphenResult, hyphenErr = executeSearch(client, host, hyphenQ, 1, searchPageSize)
		})
	}

	// Primary content search runs on the main goroutine.
	primaryItems, _, primaryErr = fetchPrimaryPages(client, host, primaryQ, page, limit)
	wg.Wait()

	if primaryErr != nil {
		return nil, primaryErr
	}

	// Merge: path-matched > hyphen-matched > owner-matched > primary content.
	var merged []codeSearchItem

	if pathErr == nil && pathResult != nil {
		merged = append(merged, pathResult.Items...)
	}
	if hasSpaces && hyphenErr == nil && hyphenResult != nil {
		merged = append(merged, hyphenResult.Items...)
	}
	if ownerErr == nil && ownerResult != nil {
		merged = append(merged, ownerResult.Items...)
	}
	merged = append(merged, primaryItems...)

	return deduplicateResults(merged), nil
}

// noResults returns an empty JSON array for exporters or a no-results error.
func noResults(opts *SearchOptions, msg string) error {
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, []skillResult{})
	}
	return cmdutil.NewNoResultsError(msg)
}

// truncateForProcessing caps the working set before expensive enrichment.
// Each skill in the working set triggers a blob fetch (description) and
// potentially a repo fetch (stars), so keeping this small matters for
// performance. Pre-ranking ensures the best candidates are at the top.
func truncateForProcessing(skills []skillResult, page, limit int) []skillResult {
	maxToProcess := max(page*limit*3, limit*3)
	if len(skills) > maxToProcess {
		return skills[:maxToProcess]
	}
	return skills
}

// enrichSkills fetches descriptions and star counts concurrently.
// Each function collects results into a map; merges happen after both complete
// to avoid concurrent writes to the shared skills slice.
func enrichSkills(client *api.Client, host string, skills []skillResult) {
	var descMap map[int]string
	var starsMap map[int]int

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		descMap = fetchDescriptions(client, host, skills)
	}()
	go func() {
		defer wg.Done()
		starsMap = fetchRepoStars(client, host, skills)
	}()
	wg.Wait()

	for i := range skills {
		if desc, ok := descMap[i]; ok {
			skills[i].Description = desc
		}
		if stars, ok := starsMap[i]; ok {
			skills[i].Stars = stars
		}
	}
}

// paginate slices results to the requested page window.
func paginate(skills []skillResult, page, limit int) ([]skillResult, int) {
	total := len(skills)
	totalPages := (total + limit - 1) / limit
	start := (page - 1) * limit
	if start >= total {
		return nil, totalPages
	}
	end := min(start+limit, total)
	return skills[start:end], totalPages
}

// deduplicateByName caps the number of results with the same qualified skill
// name. Since results are pre-sorted by relevance score, the first occurrences
// are the best instances. This prevents aggregator repos (which copy
// popular skills verbatim) from flooding results while still showing
// a few alternative sources. Namespaced skills (e.g. "author/skill") are
// treated as distinct from bare names.
func deduplicateByName(skills []skillResult) []skillResult {
	const maxPerName = 3
	counts := make(map[string]int)
	var result []skillResult
	for _, s := range skills {
		key := strings.ToLower(s.qualifiedName())
		if counts[key] >= maxPerName {
			continue
		}
		counts[key]++
		result = append(result, s)
	}
	return result
}

// renderResults handles all output modes: JSON, interactive picker, or table.
func renderResults(opts *SearchOptions, skills []skillResult, totalPages int) error {
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, skills)
	}

	cs := opts.IO.ColorScheme()
	header := fmt.Sprintf("\n%s Showing %s matching %q",
		cs.SuccessIcon(),
		text.Pluralize(len(skills), "skill"),
		opts.Query,
	)
	if totalPages > 1 {
		header += fmt.Sprintf(" (page %d/%d)", opts.Page, totalPages)
	}

	if opts.IO.CanPrompt() {
		fmt.Fprintln(opts.IO.ErrOut, header)
		if opts.Page < totalPages {
			fmt.Fprintf(opts.IO.ErrOut, "Use --page %d for more results.\n", opts.Page+1)
		}
		return promptInstall(opts, skills)
	}

	// Non-interactive mode: render table.
	if opts.IO.IsStdoutTTY() {
		fmt.Fprintln(opts.IO.Out, header)
		fmt.Fprintln(opts.IO.Out)
	}

	if err := renderTable(opts.IO, skills); err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() && opts.Page < totalPages {
		fmt.Fprintf(opts.IO.ErrOut, "\nUse --page %d for more results.\n", opts.Page+1)
	}

	return nil
}

// renderTable outputs a formatted table of skill results.
func renderTable(io *iostreams.IOStreams, skills []skillResult) error {
	isTTY := io.IsStdoutTTY()
	tw := io.TerminalWidth()
	descWidth := max(tw-70, 20)

	table := tableprinter.New(io, tableprinter.WithHeader("REPOSITORY", "SKILL", "DESCRIPTION", "STARS"))
	for _, s := range skills {
		table.AddField(s.Repo)
		table.AddField(s.qualifiedName())
		desc := s.Description
		if isTTY {
			desc = text.Truncate(descWidth, desc)
		}
		table.AddField(desc)
		table.AddField(formatStars(s.Stars))
		table.EndRow()
	}
	return table.Render()
}

// promptInstall shows a multi-select picker for the user to choose skills
// to install from the search results, then runs the install command for each.
func promptInstall(opts *SearchOptions, skills []skillResult) error {
	fmt.Fprintln(opts.IO.ErrOut)

	cs := opts.IO.ColorScheme()

	// Reserve space for the checkbox UI prefix ("[ ] ") and the description
	// indent ("\n       " = 7 chars), then use the remaining terminal width.
	tw := opts.IO.TerminalWidth()
	descWidth := max(tw-11, 30)

	options := make([]string, len(skills))
	for i, s := range skills {
		starStr := ""
		if s.Stars > 0 {
			starStr = "  " + cs.Muted("★ "+formatStars(s.Stars))
		}
		descStr := ""
		if s.Description != "" {
			desc := strings.Join(strings.Fields(s.Description), " ")
			descStr = "\n       " + cs.Muted(text.Truncate(descWidth, desc))
		}
		options[i] = s.qualifiedName() + "  " + cs.Muted(s.Repo) + starStr + descStr
	}

	indices, err := opts.Prompter.MultiSelect(
		"Select skills to install:",
		nil,
		options,
	)
	if err != nil {
		return err
	}

	if len(indices) == 0 {
		return nil
	}

	opts.Telemetry.Record(ghtelemetry.Event{
		Type: "skill_search_install",
		Measures: ghtelemetry.Measures{
			"install_count": int64(len(indices)),
		},
	})

	// Prompt for target agent host (once for all selected skills)
	hostNames := registry.AgentNames()
	hostIdx, err := opts.Prompter.Select("Select target agent:", "", hostNames)
	if err != nil {
		return err
	}
	host := registry.Agents[hostIdx]

	// Prompt for installation scope
	scopeIdx, err := opts.Prompter.Select("Installation scope:", "", registry.ScopeLabels(""))
	if err != nil {
		return err
	}
	scope := string(registry.ScopeProject)
	if scopeIdx == 1 {
		scope = string(registry.ScopeUser)
	}

	for _, idx := range indices {
		s := skills[idx]
		displayName := s.qualifiedName()
		fmt.Fprintf(opts.IO.ErrOut, "\n%s Installing %s from %s...\n",
			cs.Blue("::"), displayName, s.Repo)

		// Use the repo-relative directory path (e.g. "skills/author/name")
		// for disambiguation when installing namespaced skills, so the
		// install command can resolve the exact skill without ambiguity.
		installArg := s.SkillName
		if s.Namespace != "" {
			installArg = strings.TrimSuffix(s.Path, "/SKILL.md")
		}

		//nolint:gosec // arguments are from user-selected search results, not arbitrary input
		cmd := exec.Command(opts.ExecutablePath, "skills", "install", s.Repo, installArg,
			"--agent", host.ID, "--scope", scope)
		cmd.Stdin = os.Stdin
		cmd.Stdout = opts.IO.Out
		cmd.Stderr = opts.IO.ErrOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "%s Failed to install %s from %s: %s\n",
				cs.Red("!"), displayName, s.Repo, err)
		}
	}

	return nil
}

// relevanceScore computes a numeric ranking score for a search result.
// Higher scores rank first. Signals (in priority order):
//   - Exact skill name match (3 000 points)
//   - Partial skill name match (1 000 points)
//   - Namespace match (500 points)
//   - Description contains query (100 points)
//   - Repository stars (sqrt bonus, ~2 400 for 6k stars)
func relevanceScore(s skillResult, query string) int {
	term := strings.ToLower(query)
	termHyphen := strings.ReplaceAll(term, " ", "-")
	score := 0

	// Name match. Normalize spaces to hyphens since skill directory names
	// use hyphens as word separators (e.g. query "mcp apps" > "mcp-apps").
	skillLower := strings.ToLower(s.SkillName)
	if skillLower == term || skillLower == termHyphen {
		score += 3_000
	} else if strings.Contains(skillLower, term) || strings.Contains(skillLower, termHyphen) {
		score += 1_000
	}

	// Namespace match.
	if s.Namespace != "" && strings.Contains(strings.ToLower(s.Namespace), term) {
		score += 500
	}

	// Description match.
	if strings.Contains(strings.ToLower(s.Description), term) {
		score += 100
	}

	// Stars bonus: use √n scaling so popular repos rank meaningfully higher
	// without completely drowning out less-popular but more relevant results.
	if s.Stars > 0 {
		score += int(math.Sqrt(float64(s.Stars)) * 30)
	}

	return score
}

// filterByRelevance removes results that are not meaningfully related to
// the query. A result is kept if the query term appears in the skill name,
// the namespace, the YAML description, or the repository owner or name.
func filterByRelevance(skills []skillResult, query string) []skillResult {
	queryTerm := strings.ToLower(query)
	termHyphen := strings.ReplaceAll(queryTerm, " ", "-")

	filtered := skills[:0] // reuse backing array
	for _, s := range skills {
		nameLower := strings.ToLower(s.SkillName)
		namespaceLower := strings.ToLower(s.Namespace)
		descLower := strings.ToLower(s.Description)
		ownerLower := strings.ToLower(s.Owner)
		repoLower := strings.ToLower(s.RepoName)

		if strings.Contains(nameLower, queryTerm) ||
			strings.Contains(nameLower, termHyphen) ||
			strings.Contains(namespaceLower, queryTerm) ||
			strings.Contains(descLower, queryTerm) ||
			strings.Contains(ownerLower, queryTerm) ||
			strings.Contains(repoLower, queryTerm) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// rankByRelevance sorts results by multi-signal score, highest first.
func rankByRelevance(skills []skillResult, query string) {
	sort.SliceStable(skills, func(i, j int) bool {
		return relevanceScore(skills[i], query) > relevanceScore(skills[j], query)
	})
}

// couldBeOwner returns true if s looks like a valid GitHub username/org.
// GitHub usernames: 1-39 chars, alphanumeric or hyphen, no leading/trailing hyphens.
func couldBeOwner(s string) bool {
	if len(s) == 0 || len(s) > 39 {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			continue
		case c == '-':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// isRateLimitError checks whether err is a GitHub API rate-limit response.
// Per GitHub docs, a rate limit is indicated by:
//   - HTTP 429 (always a rate limit)
//   - HTTP 403 with x-ratelimit-remaining: 0 (primary rate limit)
//   - HTTP 403 with a retry-after header (secondary rate limit)
func isRateLimitError(err error) bool {
	var httpErr api.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.StatusCode == 429 {
		return true
	}
	if httpErr.StatusCode == 403 {
		if httpErr.Headers.Get("x-ratelimit-remaining") == "0" {
			return true
		}
		if httpErr.Headers.Get("retry-after") != "" {
			return true
		}
	}
	return false
}

// rateLimitErrorMessage returns a user-friendly message for rate-limit errors.
const rateLimitErrorMessage = "GitHub API rate limit exceeded. Please wait a minute and try again."

// executeSearch performs a single GitHub Code Search API call.
func executeSearch(client *api.Client, host, query string, page, pageSize int) (*codeSearchResult, error) {
	apiPath, err := safeurl.JoinPath("search", "code")
	if err != nil {
		return nil, err
	}
	apiPath.SetQuery("q", query)
	apiPath.SetQuery("per_page", strconv.Itoa(pageSize))
	apiPath.SetQuery("page", strconv.Itoa(page))
	var result codeSearchResult
	err = client.REST(host, "GET", apiPath.String(), nil, &result)
	if err != nil && isRateLimitError(err) {
		return nil, fmt.Errorf("%s", rateLimitErrorMessage)
	}
	return &result, err
}

// fetchPrimaryPages fetches enough API pages from GitHub Code Search to
// cover the requested display page, accounting for filtering losses.
func fetchPrimaryPages(client *api.Client, host, query string, displayPage, displayLimit int) ([]codeSearchItem, int, error) {
	// Over-fetch to account for deduplication + filtering losses.
	// The Code Search API is rate-limited at 10 req/min, so we keep
	// page fetching conservative. Two pages (200 results) provides a
	// good buffer for typical filter rates while staying well within
	// the rate-limit budget.
	needed := displayPage * displayLimit * 3
	numPages := max((needed+searchPageSize-1)/searchPageSize, 1)
	maxAPIPages := maxResults / searchPageSize
	if numPages > maxAPIPages {
		numPages = maxAPIPages
	}

	var allItems []codeSearchItem
	var totalCount int
	for p := 1; p <= numPages; p++ {
		result, err := executeSearch(client, host, query, p, searchPageSize)
		if err != nil {
			if p == 1 {
				return nil, 0, err
			}
			break // partial results from earlier pages are OK
		}
		allItems = append(allItems, result.Items...)
		totalCount = result.TotalCount
		if len(result.Items) < searchPageSize {
			break // no more results available
		}
	}
	return allItems, totalCount, nil
}

// deduplicateResults extracts unique (repo, namespace, skill name) triples from code search hits.
func deduplicateResults(items []codeSearchItem) []skillResult {
	// skillResultKey is a typed map key that deduplicates by (repo, namespace,
	// skill name). All fields are lowercased for case-insensitive comparison.
	type skillResultKey struct {
		repo      string
		namespace string
		skillName string
	}
	seen := make(map[skillResultKey]struct{})
	var results []skillResult

	for _, item := range items {
		skillName, namespace := extractSkillInfo(item.Path)
		if skillName == "" {
			continue
		}
		key := skillResultKey{
			repo:      strings.ToLower(item.Repository.FullName),
			namespace: strings.ToLower(namespace),
			skillName: strings.ToLower(skillName),
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		owner, repoName := splitRepo(item.Repository.FullName)
		results = append(results, skillResult{
			Repo:      item.Repository.FullName,
			Owner:     owner,
			RepoName:  repoName,
			SkillName: skillName,
			Namespace: namespace,
			Path:      item.Path,
			BlobSHA:   item.SHA,
		})
	}

	return results
}

// splitRepo splits "owner/repo" into its components.
func splitRepo(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return fullName, ""
	}
	return parts[0], parts[1]
}

// fetchDescriptions fetches SKILL.md frontmatter descriptions concurrently
// for all search results. Each result may come from a different repo.
func fetchDescriptions(client *api.Client, host string, skills []skillResult) map[int]string {
	const maxWorkers = 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	descs := make(map[int]string)

	for i := range skills {
		if skills[i].BlobSHA == "" {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			content, err := discovery.FetchBlob(client, host, skills[idx].Owner, skills[idx].RepoName, skills[idx].BlobSHA)
			if err != nil {
				return
			}
			result, err := frontmatter.Parse(content.Raw())
			if err != nil {
				return
			}

			mu.Lock()
			descs[idx] = result.Metadata.Description
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	return descs
}

// extractSkillInfo derives the skill name and namespace from a SKILL.md path,
// but only if the path matches a known skill convention. Returns empty strings
// for non-conforming paths.
func extractSkillInfo(filePath string) (name, namespace string) {
	return discovery.MatchSkillPath(filePath)
}

// formatStars formats a star count for display (e.g. 1700 > "1.7k").
// TODO kw: Could be swapped for go-humanize.
func formatStars(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// repoInfo holds the subset of repository metadata we fetch for ranking.
type repoInfo struct {
	StargazersCount int `json:"stargazers_count"`
}

// fetchRepoStars fetches stargazer counts for each unique repository in
// the result set, using bounded concurrency.
func fetchRepoStars(client *api.Client, host string, skills []skillResult) map[int]int {
	const maxWorkers = 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	repoStars := make(map[string]int)
	seen := make(map[string]bool)

	for _, s := range skills {
		if seen[s.Repo] {
			continue
		}
		seen[s.Repo] = true

		wg.Add(1)
		go func(owner, repo, fullName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			apiPath, err := safeurl.JoinPath("repos", owner, repo)
			if err != nil {
				return
			}
			var info repoInfo
			if err := client.REST(host, "GET", apiPath.String(), nil, &info); err != nil {
				return
			}
			mu.Lock()
			repoStars[fullName] = info.StargazersCount
			mu.Unlock()
		}(s.Owner, s.RepoName, s.Repo)
	}
	wg.Wait()

	result := make(map[int]int, len(skills))
	for i, s := range skills {
		if stars, ok := repoStars[s.Repo]; ok {
			result[i] = stars
		}
	}
	return result
}
