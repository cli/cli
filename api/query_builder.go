package api

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/set"
)

func squeeze(r rune) rune {
	switch r {
	case '\n', '\t':
		return -1
	default:
		return r
	}
}

func shortenQuery(q string) string {
	return strings.Map(squeeze, q)
}

var issueComments = shortenQuery(`
	comments(first: 100) {
		nodes {
			id,
			author{login,...on User{id,name}},
			authorAssociation,
			body,
			createdAt,
			includesCreatedEdit,
			isMinimized,
			minimizedReason,
			reactionGroups{content,users{totalCount}},
			url,
			viewerDidAuthor
		},
		pageInfo{hasNextPage,endCursor},
		totalCount
	}
`)

var issueCommentLast = shortenQuery(`
	comments(last: 1) {
		nodes {
			author{login,...on User{id,name}},
			authorAssociation,
			body,
			createdAt,
			includesCreatedEdit,
			isMinimized,
			minimizedReason,
			reactionGroups{content,users{totalCount}}
		},
		totalCount
	}
`)

var prReviewRequests = shortenQuery(`
	reviewRequests(first: 100) {
		nodes {
			requestedReviewer {
				__typename,
				...on User{login},
				...on Team{
					organization{login}
					name,
					slug
				}
			}
		}
	}
`)

var prReviews = shortenQuery(`
	reviews(first: 100) {
		nodes {
			id,
			author{login},
			authorAssociation,
			submittedAt,
			body,
			state,
			commit{oid},
			reactionGroups{content,users{totalCount}}
		}
		pageInfo{hasNextPage,endCursor}
		totalCount
	}
`)

var prLatestReviews = shortenQuery(`
	latestReviews(first: 100) {
		nodes {
			author{login},
			authorAssociation,
			submittedAt,
			body,
			state
		}
	}
`)

var prFiles = shortenQuery(`
	files(first: 100) {
		nodes {
			additions,
			deletions,
			path
		}
	}
`)

var prCommits = shortenQuery(`
	commits(first: 100) {
		nodes {
			commit {
				authors(first:100) {
					nodes {
						name,
						email,
						user{id,login}
					}
				},
				messageHeadline,
				messageBody,
				oid,
				committedDate,
				authoredDate
			}
		}
	}
`)

func StatusCheckRollupGraphQL(after string) string {
	var afterClause string
	if after != "" {
		afterClause = ",after:" + after
	}
	return fmt.Sprintf(shortenQuery(`
	statusCheckRollup: commits(last: 1) {
		nodes {
			commit {
				statusCheckRollup {
					contexts(first:100%s) {
						nodes {
							__typename
							...on StatusContext {
								context,
								state,
								targetUrl,
								createdAt
							},
							...on CheckRun {
								name,
								checkSuite{workflowRun{workflow{name}}},
								status,
								conclusion,
								startedAt,
								completedAt,
								detailsUrl
							}
						},
						pageInfo{hasNextPage,endCursor}
					}
				}
			}
		}
	}`), afterClause)
}

func RequiredStatusCheckRollupGraphQL(prID, after string) string {
	var afterClause string
	if after != "" {
		afterClause = ",after:" + after
	}
	return fmt.Sprintf(shortenQuery(`
	statusCheckRollup: commits(last: 1) {
		nodes {
			commit {
				statusCheckRollup {
					contexts(first:100%[1]s) {
						nodes {
							__typename
							...on StatusContext {
								context,
								state,
								targetUrl,
								createdAt,
								isRequired(pullRequestId: %[2]s)
							},
							...on CheckRun {
								name,
								checkSuite{workflowRun{workflow{name}}},
								status,
								conclusion,
								startedAt,
								completedAt,
								detailsUrl,
								isRequired(pullRequestId: %[2]s)
							}
						},
						pageInfo{hasNextPage,endCursor}
					}
				}
			}
		}
	}`), afterClause, prID)
}

var IssueFields = []string{
	"assignees",
	"author",
	"body",
	"closed",
	"comments",
	"createdAt",
	"closedAt",
	"id",
	"labels",
	"milestone",
	"number",
	"projectCards",
	"projectItems",
	"reactionGroups",
	"state",
	"title",
	"updatedAt",
	"url",
}

var PullRequestFields = append(IssueFields,
	"additions",
	"baseRefName",
	"changedFiles",
	"commits",
	"deletions",
	"files",
	"headRefName",
	"headRefOid",
	"headRepository",
	"headRepositoryOwner",
	"isCrossRepository",
	"isDraft",
	"latestReviews",
	"maintainerCanModify",
	"mergeable",
	"mergeCommit",
	"mergedAt",
	"mergedBy",
	"mergeStateStatus",
	"potentialMergeCommit",
	"reviewDecision",
	"reviewRequests",
	"reviews",
	"statusCheckRollup",
)

// IssueGraphQL constructs a GraphQL query fragment for a set of issue fields.
func IssueGraphQL(fields []string) string {
	var q []string
	for _, field := range fields {
		switch field {
		case "author":
			q = append(q, `author{login,...on User{id,name}}`)
		case "mergedBy":
			q = append(q, `mergedBy{login,...on User{id,name}}`)
		case "headRepositoryOwner":
			q = append(q, `headRepositoryOwner{id,login,...on User{name}}`)
		case "headRepository":
			q = append(q, `headRepository{id,name}`)
		case "assignees":
			q = append(q, `assignees(first:100){nodes{id,login,name},totalCount}`)
		case "labels":
			q = append(q, `labels(first:100){nodes{id,name,description,color},totalCount}`)
		case "projectCards":
			q = append(q, `projectCards(first:100){nodes{project{name}column{name}},totalCount}`)
		case "projectItems":
			q = append(q, `projectItems(first:100){nodes{id, project{id,title}},totalCount}`)
		case "milestone":
			q = append(q, `milestone{number,title,description,dueOn}`)
		case "reactionGroups":
			q = append(q, `reactionGroups{content,users{totalCount}}`)
		case "mergeCommit":
			q = append(q, `mergeCommit{oid}`)
		case "potentialMergeCommit":
			q = append(q, `potentialMergeCommit{oid}`)
		case "comments":
			q = append(q, issueComments)
		case "lastComment": // pseudo-field
			q = append(q, issueCommentLast)
		case "reviewRequests":
			q = append(q, prReviewRequests)
		case "reviews":
			q = append(q, prReviews)
		case "latestReviews":
			q = append(q, prLatestReviews)
		case "files":
			q = append(q, prFiles)
		case "commits":
			q = append(q, prCommits)
		case "lastCommit": // pseudo-field
			q = append(q, `commits(last:1){nodes{commit{oid}}}`)
		case "commitsCount": // pseudo-field
			q = append(q, `commits{totalCount}`)
		case "requiresStrictStatusChecks": // pseudo-field
			q = append(q, `baseRef{branchProtectionRule{requiresStrictStatusChecks}}`)
		case "statusCheckRollup":
			q = append(q, StatusCheckRollupGraphQL(""))
		default:
			q = append(q, field)
		}
	}
	return strings.Join(q, ",")
}

// PullRequestGraphQL constructs a GraphQL query fragment for a set of pull request fields.
// It will try to sanitize the fields to just those available on pull request.
func PullRequestGraphQL(fields []string) string {
	invalidFields := []string{"isPinned", "stateReason"}
	s := set.NewStringSet()
	s.AddValues(fields)
	s.RemoveValues(invalidFields)
	return IssueGraphQL(s.ToSlice())
}

var RepositoryFields = []string{
	"id",
	"name",
	"nameWithOwner",
	"owner",
	"parent",
	"templateRepository",
	"description",
	"homepageUrl",
	"openGraphImageUrl",
	"usesCustomOpenGraphImage",
	"url",
	"sshUrl",
	"mirrorUrl",
	"securityPolicyUrl",

	"createdAt",
	"pushedAt",
	"updatedAt",

	"isBlankIssuesEnabled",
	"isSecurityPolicyEnabled",
	"hasIssuesEnabled",
	"hasProjectsEnabled",
	"hasWikiEnabled",
	"hasDiscussionsEnabled",
	"mergeCommitAllowed",
	"squashMergeAllowed",
	"rebaseMergeAllowed",

	"forkCount",
	"stargazerCount",
	"watchers",
	"issues",
	"pullRequests",

	"codeOfConduct",
	"contactLinks",
	"defaultBranchRef",
	"deleteBranchOnMerge",
	"diskUsage",
	"fundingLinks",
	"isArchived",
	"isEmpty",
	"isFork",
	"isInOrganization",
	"isMirror",
	"isPrivate",
	"isTemplate",
	"isUserConfigurationRepository",
	"licenseInfo",
	"viewerCanAdminister",
	"viewerDefaultCommitEmail",
	"viewerDefaultMergeMethod",
	"viewerHasStarred",
	"viewerPermission",
	"viewerPossibleCommitEmails",
	"viewerSubscription",

	"repositoryTopics",
	"primaryLanguage",
	"languages",
	"issueTemplates",
	"pullRequestTemplates",
	"labels",
	"milestones",
	"latestRelease",

	"assignableUsers",
	"mentionableUsers",
	"projects",

	// "branchProtectionRules", // too complex to expose
	// "collaborators", // does it make sense to expose without affiliation filter?
}

func RepositoryGraphQL(fields []string) string {
	var q []string
	for _, field := range fields {
		switch field {
		case "codeOfConduct":
			q = append(q, "codeOfConduct{key,name,url}")
		case "contactLinks":
			q = append(q, "contactLinks{about,name,url}")
		case "fundingLinks":
			q = append(q, "fundingLinks{platform,url}")
		case "licenseInfo":
			q = append(q, "licenseInfo{key,name,nickname}")
		case "owner":
			q = append(q, "owner{id,login}")
		case "parent":
			q = append(q, "parent{id,name,owner{id,login}}")
		case "templateRepository":
			q = append(q, "templateRepository{id,name,owner{id,login}}")
		case "repositoryTopics":
			q = append(q, "repositoryTopics(first:100){nodes{topic{name}}}")
		case "issueTemplates":
			q = append(q, "issueTemplates{name,title,body,about}")
		case "pullRequestTemplates":
			q = append(q, "pullRequestTemplates{body,filename}")
		case "labels":
			q = append(q, "labels(first:100){nodes{id,color,name,description}}")
		case "languages":
			q = append(q, "languages(first:100){edges{size,node{name}}}")
		case "primaryLanguage":
			q = append(q, "primaryLanguage{name}")
		case "latestRelease":
			q = append(q, "latestRelease{publishedAt,tagName,name,url}")
		case "milestones":
			q = append(q, "milestones(first:100,states:OPEN){nodes{number,title,description,dueOn}}")
		case "assignableUsers":
			q = append(q, "assignableUsers(first:100){nodes{id,login,name}}")
		case "mentionableUsers":
			q = append(q, "mentionableUsers(first:100){nodes{id,login,name}}")
		case "projects":
			q = append(q, "projects(first:100,states:OPEN){nodes{id,name,number,body,resourcePath}}")
		case "watchers":
			q = append(q, "watchers{totalCount}")
		case "issues":
			q = append(q, "issues(states:OPEN){totalCount}")
		case "pullRequests":
			q = append(q, "pullRequests(states:OPEN){totalCount}")
		case "defaultBranchRef":
			q = append(q, "defaultBranchRef{name}")
		default:
			q = append(q, field)
		}
	}
	return strings.Join(q, ",")
}
