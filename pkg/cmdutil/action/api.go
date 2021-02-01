package action

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
)

// https://docs.github.com/en/rest/overview/api-previews
func ActionApiPreviews() carapace.Action {
	return carapace.ActionValuesDescribed(
		"wyandotte", "Migrations",
		"ant-man", "Enhanced deployments",
		"squirrel-girl", "Reactions",
		"mockingbird", "Timeline",
		"inertia", "Projects",
		"cloak", "Commit search",
		"mercy", "Repository topics",
		"scarlet-witch", "Codes of conduct",
		"zzzax", "Require signed commits",
		"luke-cage", "Require multiple approving reviews",
		"starfox", "Project card details",
		"fury", "GitHub App Manifests",
		"flash", "Deployment statuses",
		"surtur", "Repository creation permissions",
		"corsair", "Content attachments",
		"switcheroo", "Enable and disable Pages",
		"groot", "List branches or pull requests for a commit",
		"dorian", "Enable or disable vulnerability alerts for a repository",
		"lydian", "Update a pull request branch",
		"london", "Enable or disable automated security fixes",
		"baptiste", "Create and use repository templates",
		"nebula", "New visibility parameter for the Repositories API",
	)
}

func ActionApiV3Paths(cmd *cobra.Command) carapace.Action {
	return carapace.ActionMultiParts("/", func(c carapace.Context) carapace.Action {
		placeholder := regexp.MustCompile(`{(.*)}`)
		matchedData := make(map[string]string)
		matchedSegments := make(map[string]bool)
		staticMatches := make(map[int]bool)

	path:
		for _, path := range v3Paths {
			segments := strings.Split(path, "/")
		segment:
			for index, segment := range segments {
				if index > len(c.Parts)-1 {
					break segment
				} else {
					if segment != c.Parts[index] {
						if !placeholder.MatchString(segment) {
							continue path // skip this path as it doesn't match and is not a placeholder
						} else {
							matchedData[segment] = c.Parts[index] // store entered data for placeholder (overwrite if duplicate)
						}
					} else {
						staticMatches[index] = true // static segment matches so placeholders should be ignored for this index
					}
				}
			}

			if len(segments) < len(c.Parts)+1 {
				continue path // skip path as it is shorter than what was entered (must be after staticMatches being set)
			}

			for key := range staticMatches {
				if segments[key] != c.Parts[key] {
					continue path // skip this path as it has a placeholder where a static segment was matched
				}
			}
			matchedSegments[segments[len(c.Parts)]] = true // store segment as path matched so far and this is currently being completed
		}

		actions := make([]carapace.InvokedAction, 0, len(matchedSegments))
		for key := range matchedSegments {
			switch key {
			// TODO completion for other placeholders
			case "{archive_format}":
				actions = append(actions, carapace.ActionValues("zip").Invoke(c))
			case "{artifact_id}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionWorkflowArtifactIds(cmd, "").Invoke(c))
			case "{assignee}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionAssignableUsers(cmd).Invoke(c))
			case "{branch}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionBranches(cmd).Invoke(c))
			case "{gist_id}":
				actions = append(actions, ActionGists(cmd).Invoke(c))
			case "{gitignore_name}":
				actions = append(actions, ActionGitignoreTemplates(cmd).Invoke(c))
			case "{issue_number}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionIssues(cmd, IssueOpts{Open: true, Closed: true}).Invoke(c))
			case "{owner}":
				if strings.HasPrefix(c.CallbackValue, ":") {
					actions = append(actions, carapace.ActionValues(":owner").Invoke(c))
				} else {
					actions = append(actions, ActionUsers(cmd, UserOpts{Users: true, Organizations: true}).Invoke(c))
				}
			case "{org}":
				actions = append(actions, ActionUsers(cmd, UserOpts{Organizations: true}).Invoke(c))
			case "{package_type}":
				actions = append(actions, ActionPackageTypes().Invoke(c))
			case "{pull_number}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionPullRequests(cmd, PullRequestOpts{Open: true, Closed: true, Merged: true}).Invoke(c))
			case "{repo}":
				if strings.HasPrefix(c.CallbackValue, ":") {
					actions = append(actions, carapace.ActionValues(":repo").Invoke(c))
				} else {
					actions = append(actions, ActionRepositories(cmd, matchedData["{owner}"], c.CallbackValue).Invoke(c))
				}
			case "{tag}": // only used with releases
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionReleases(cmd).Invoke(c))
			case "{template_owner}": // ignore this as it is already provided by `{owner}`
			case "{template_repo}": // ignore this as it is already provided by `{repo}`
			case "{username}":
				actions = append(actions, ActionUsers(cmd, UserOpts{Users: true}).Invoke(c))
			case "{workflow_id}":
				fakeRepoFlag(cmd, matchedData["{owner}"], matchedData["{repo}"])
				actions = append(actions, ActionWorkflows(cmd, WorkflowOpts{Enabled: true, Disabled: true, Id: true}).Invoke(c))
			default:
				// static value or placeholder not yet handled
				actions = append(actions, carapace.ActionValues(key).Invoke(c))
			}
		}
		switch len(actions) {
		case 0:
			return carapace.ActionValues()
		case 1:
			return actions[0].ToA()
		default:
			return actions[0].Merge(actions[1:]...).ToA()
		}
	})
}

func fakeRepoFlag(cmd *cobra.Command, owner, repo string) {
	cmd.Flags().String("repo", fmt.Sprintf("%v/%v", owner, repo), "fake repo flag")
	cmd.Flag("repo").Changed = true
}
