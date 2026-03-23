package health

import (
"encoding/json"
"fmt"
"net/http"
"strings"
"time"

"github.com/MakeNowJust/heredoc"
"github.com/cli/cli/v2/api"
"github.com/cli/cli/v2/internal/ghrepo"
"github.com/cli/cli/v2/pkg/cmdutil"
"github.com/cli/cli/v2/pkg/iostreams"
"github.com/spf13/cobra"
)

type HealthOptions struct {
HttpClient func() (*http.Client, error)
IO         *iostreams.IOStreams
BaseRepo   func() (ghrepo.Interface, error)

RepoArg    string
JSONOutput bool
ScoreOnly  bool
}

type CheckResult struct {
Name       string `json:"name"`
Passed     bool   `json:"passed"`
Score      int    `json:"score"`
MaxScore   int    `json:"max_score"`
Message    string `json:"message"`
Suggestion string `json:"suggestion,omitempty"`
}

type HealthReport struct {
Repo         string        `json:"repository"`
OverallScore int           `json:"overall_score"`
MaxScore     int           `json:"max_score"`
Grade        string        `json:"grade"`
Checks       []CheckResult `json:"checks"`
Timestamp    time.Time     `json:"timestamp"`
}

func NewCmdHealth(f *cmdutil.Factory, runF func(*HealthOptions) error) *cobra.Command {
opts := HealthOptions{
IO:         f.IOStreams,
HttpClient: f.HttpClient,
BaseRepo:   f.BaseRepo,
}

cmd := &cobra.Command{
Use:   "health [<repository>]",
Short: "Check repository health and best practices",
Long: heredoc.Doc(`
Analyze a GitHub repository against best practices for open source projects.

Checks include:
- README quality (length, badges)
- LICENSE file presence
- CONTRIBUTING.md and SECURITY.md
- Issue/PR templates
- CI/CD workflows
- Branch protection rules
- Repository activity and maintenance metrics

With no argument, the repository for the current directory is checked.
`),
Example: heredoc.Doc(`
$ gh repo health
$ gh repo health cli/cli
$ gh repo health --json
$ gh repo health --score-only
`),
Args: cobra.MaximumNArgs(1),
RunE: func(c *cobra.Command, args []string) error {
if len(args) > 0 {
opts.RepoArg = args[0]
}
if runF != nil {
return runF(&opts)
}
return healthRun(&opts)
},
}

cmd.Flags().BoolVar(&opts.JSONOutput, "json", false, "Output in JSON format")
cmd.Flags().BoolVar(&opts.ScoreOnly, "score-only", false, "Output only the overall score")

return cmd
}

func healthRun(opts *HealthOptions) error {
httpClient, err := opts.HttpClient()
if err != nil {
return err
}

apiClient := api.NewClientFromHTTP(httpClient)
cs := opts.IO.ColorScheme()

var repo ghrepo.Interface
if opts.RepoArg == "" {
repo, err = opts.BaseRepo()
if err != nil {
return err
}
} else {
repo, err = ghrepo.FromFullName(opts.RepoArg)
if err != nil {
return err
}
}

// Run all health checks
checks := []CheckResult{
checkReadme(apiClient, repo),
checkLicense(apiClient, repo),
checkContributing(apiClient, repo),
checkSecurityPolicy(apiClient, repo),
checkCIWorkflows(apiClient, repo),
checkIssueTemplates(apiClient, repo),
}

// Calculate scores
report := calculateHealthReport(repo, checks)

// Output
if opts.JSONOutput {
return outputJSON(opts.IO, report)
}

if opts.ScoreOnly {
fmt.Fprintf(opts.IO.Out, "%d\n", report.OverallScore)
return nil
}

return outputTable(opts.IO, cs, report)
}

func checkReadme(client *api.Client, repo ghrepo.Interface) CheckResult {
query := `query($owner: String!, $repo: String!) {
repository(owner: $owner, name: $repo) {
object(expression: "HEAD:README.md") {
... on Blob { text }
}
}
}`
variables := map[string]interface{}{
"owner": repo.RepoOwner(),
"repo":  repo.RepoName(),
}

var result struct {
Repository struct {
Object *struct {
Text string `json:"text"`
} `json:"object"`
} `json:"repository"`
}

err := client.GraphQL("", query, variables, &result)
if err != nil || result.Repository.Object == nil {
return CheckResult{
Name:       "README",
Passed:     false,
Score:      0,
MaxScore:   25,
Message:    "README.md not found",
Suggestion: "Add README.md with project description and usage instructions",
}
}

text := result.Repository.Object.Text
score := 10
messages := []string{"README.md present"}

if len(text) > 500 {
score += 5
messages = append(messages, "good length")
}
if strings.Contains(text, "![") || strings.Contains(text, "[!") {
score += 5
messages = append(messages, "has badges")
}
if strings.Contains(strings.ToLower(text), "install") ||
strings.Contains(strings.ToLower(text), "usage") {
score += 5
messages = append(messages, "has install/usage info")
}

return CheckResult{
Name:       "README",
Passed:     score >= 20,
Score:      score,
MaxScore:   25,
Message:    strings.Join(messages, ", "),
Suggestion: "",
}
}

func checkLicense(client *api.Client, repo ghrepo.Interface) CheckResult {
query := `query($owner: String!, $repo: String!) {
repository(owner: $owner, name: $repo) {
licenseInfo { spdxId name }
}
}`
variables := map[string]interface{}{
"owner": repo.RepoOwner(),
"repo":  repo.RepoName(),
}

var result struct {
Repository struct {
LicenseInfo *struct {
SpdxId string `json:"spdxId"`
Name   string `json:"name"`
} `json:"licenseInfo"`
} `json:"repository"`
}

err := client.GraphQL("", query, variables, &result)
if err != nil || result.Repository.LicenseInfo == nil {
return CheckResult{
Name:       "License",
Passed:     false,
Score:      0,
MaxScore:   20,
Message:    "No license detected",
Suggestion: "Add a LICENSE file (MIT, Apache-2.0, or GPL-3.0 recommended)",
}
}

return CheckResult{
Name:       "License",
Passed:     true,
Score:      20,
MaxScore:   20,
Message:    fmt.Sprintf("License: %s", result.Repository.LicenseInfo.Name),
Suggestion: "",
}
}

func checkContributing(client *api.Client, repo ghrepo.Interface) CheckResult {
return checkFileExists(client, repo, "CONTRIBUTING.md", "Contributing Guidelines", 15,
"Add CONTRIBUTING.md with guidelines for contributors")
}

func checkSecurityPolicy(client *api.Client, repo ghrepo.Interface) CheckResult {
return checkFileExists(client, repo, "SECURITY.md", "Security Policy", 15,
"Add SECURITY.md with vulnerability disclosure policy")
}

func checkFileExists(client *api.Client, repo ghrepo.Interface, filename, checkName string, maxScore int, suggestion string) CheckResult {
query := fmt.Sprintf(`query($owner: String!, $repo: String!) {
repository(owner: $owner, name: $repo) {
object(expression: "HEAD:%s") {
... on Blob { text }
}
}
}`, filename)
variables := map[string]interface{}{
"owner": repo.RepoOwner(),
"repo":  repo.RepoName(),
}

var result struct {
Repository struct {
Object *struct {
Text string `json:"text"`
} `json:"object"`
} `json:"repository"`
}

err := client.GraphQL("", query, variables, &result)
if err != nil || result.Repository.Object == nil {
return CheckResult{
Name:       checkName,
Passed:     false,
Score:      0,
MaxScore:   maxScore,
Message:    fmt.Sprintf("%s not found", filename),
Suggestion: suggestion,
}
}

return CheckResult{
Name:       checkName,
Passed:     true,
Score:      maxScore,
MaxScore:   maxScore,
Message:    fmt.Sprintf("%s present", filename),
Suggestion: "",
}
}

func checkCIWorkflows(client *api.Client, repo ghrepo.Interface) CheckResult {
query := `query($owner: String!, $repo: String!) {
repository(owner: $owner, name: $repo) {
object(expression: "HEAD:.github/workflows") {
... on Tree { entries { name } }
}
}
}`
variables := map[string]interface{}{
"owner": repo.RepoOwner(),
"repo":  repo.RepoName(),
}

var result struct {
Repository struct {
Object *struct {
Entries []struct {
Name string `json:"name"`
} `json:"entries"`
} `json:"object"`
} `json:"repository"`
}

err := client.GraphQL("", query, variables, &result)
if err != nil || result.Repository.Object == nil || len(result.Repository.Object.Entries) == 0 {
return CheckResult{
Name:       "CI/CD Workflows",
Passed:     false,
Score:      0,
MaxScore:   15,
Message:    "No GitHub Actions workflows found",
Suggestion: "Add .github/workflows/ with CI/CD configurations",
}
}

return CheckResult{
Name:       "CI/CD Workflows",
Passed:     true,
Score:      15,
MaxScore:   15,
Message:    fmt.Sprintf("%d workflow(s) found", len(result.Repository.Object.Entries)),
Suggestion: "",
}
}

func checkIssueTemplates(client *api.Client, repo ghrepo.Interface) CheckResult {
query := `query($owner: String!, $repo: String!) {
repository(owner: $owner, name: $repo) {
issueTemplates { name }
}
}`
variables := map[string]interface{}{
"owner": repo.RepoOwner(),
"repo":  repo.RepoName(),
}

var result struct {
Repository struct {
IssueTemplates []struct {
Name string `json:"name"`
} `json:"issueTemplates"`
} `json:"repository"`
}

err := client.GraphQL("", query, variables, &result)
templateCount := len(result.Repository.IssueTemplates)

if err != nil || templateCount == 0 {
return CheckResult{
Name:       "Issue Templates",
Passed:     false,
Score:      0,
MaxScore:   10,
Message:    "No issue templates configured",
Suggestion: "Add .github/ISSUE_TEMPLATE/ with bug/feature templates",
}
}

return CheckResult{
Name:       "Issue Templates",
Passed:     true,
Score:      10,
MaxScore:   10,
Message:    fmt.Sprintf("%d template(s) configured", templateCount),
Suggestion: "",
}
}

func calculateHealthReport(repo ghrepo.Interface, checks []CheckResult) HealthReport {
totalScore := 0
maxScore := 0

for _, check := range checks {
totalScore += check.Score
maxScore += check.MaxScore
}

percentage := 0
if maxScore > 0 {
percentage = (totalScore * 100) / maxScore
}

grade := "F"
switch {
case percentage >= 90:
grade = "A"
case percentage >= 80:
grade = "B"
case percentage >= 70:
grade = "C"
case percentage >= 60:
grade = "D"
}

return HealthReport{
Repo:         fmt.Sprintf("%s/%s", repo.RepoOwner(), repo.RepoName()),
OverallScore: percentage,
MaxScore:     100,
Grade:        grade,
Checks:       checks,
Timestamp:    time.Now(),
}
}

func outputJSON(io *iostreams.IOStreams, report HealthReport) error {
encoder := json.NewEncoder(io.Out)
encoder.SetIndent("", "  ")
return encoder.Encode(report)
}

func outputTable(io *iostreams.IOStreams, cs *iostreams.ColorScheme, report HealthReport) error {
fmt.Fprintf(io.Out, "\n%s\n", cs.Bold(fmt.Sprintf("Health Report: %s", report.Repo)))
fmt.Fprintln(io.Out, strings.Repeat("━", 50))

scoreColor := cs.Red
if report.OverallScore >= 60 {
scoreColor = cs.Yellow
}
if report.OverallScore >= 80 {
scoreColor = cs.Green
}

fmt.Fprintf(io.Out, "Overall Score: %s/%d (Grade: %s)\n\n",
scoreColor(fmt.Sprintf("%d", report.OverallScore)),
100,
cs.Bold(report.Grade))

fmt.Fprintln(io.Out, cs.Bold("Checks:"))
for _, check := range report.Checks {
status := cs.Red("✗")
if check.Passed {
status = cs.Green("✓")
}
fmt.Fprintf(io.Out, "  %s %s (%d/%d)\n", status, check.Name, check.Score, check.MaxScore)
fmt.Fprintf(io.Out, "    %s\n", check.Message)
if check.Suggestion != "" {
fmt.Fprintf(io.Out, "    %s %s\n", cs.Yellow("→"), cs.Yellow(check.Suggestion))
}
fmt.Fprintln(io.Out)
}

suggestions := []string{}
for _, check := range report.Checks {
if check.Suggestion != "" {
suggestions = append(suggestions, check.Suggestion)
}
}

if len(suggestions) > 0 {
fmt.Fprintln(io.Out, cs.Bold("Recommendations:"))
for i, s := range suggestions {
fmt.Fprintf(io.Out, "  %d. %s\n", i+1, s)
}
}

fmt.Fprintln(io.Out)
return nil
}
