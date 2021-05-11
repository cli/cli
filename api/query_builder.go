package api

import (
	"strings"
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
	comments(last: 100) {
		nodes {
			author{login},
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
	reviewRequests(last: 100) {
		nodes {
			requestedReviewer {
				__typename,
				...on User{login},
				...on Team{name}
			}
		},
		totalCount
	}
`)

var prReviews = shortenQuery(`
	reviews(last: 100) {
		nodes {
			author{login},
			authorAssociation,
			submittedAt,
			body,
			state,
			reactionGroups{content,users{totalCount}}
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

var prStatusCheckRollup = shortenQuery(`
	commits(last: 1) {
		totalCount,
		nodes {
			commit {
				oid,
				statusCheckRollup {
					contexts(last: 100) {
						nodes {
							__typename
							...on StatusContext {
								context,
								state,
								targetUrl
							},
							...on CheckRun {
								name,
								status,
								conclusion,
								startedAt,
								completedAt,
								detailsUrl
							}
						}
					}
				}
			}
		}
	}
`)

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
	"deletions",
	"files",
	"headRefName",
	"headRepository",
	"headRepositoryOwner",
	"isCrossRepository",
	"isDraft",
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

func PullRequestGraphQL(fields []string) string {
	var q []string
	for _, field := range fields {
		switch field {
		case "author":
			q = append(q, `author{login}`)
		case "mergedBy":
			q = append(q, `mergedBy{login}`)
		case "headRepositoryOwner":
			q = append(q, `headRepositoryOwner{login}`)
		case "headRepository":
			q = append(q, `headRepository{name}`)
		case "assignees":
			q = append(q, `assignees(first:100){nodes{login},totalCount}`)
		case "labels":
			q = append(q, `labels(first:100){nodes{name},totalCount}`)
		case "projectCards":
			q = append(q, `projectCards(first:100){nodes{project{name}column{name}},totalCount}`)
		case "milestone":
			q = append(q, `milestone{title}`)
		case "reactionGroups":
			q = append(q, `reactionGroups{content,users{totalCount}}`)
		case "mergeCommit":
			q = append(q, `mergeCommit{oid}`)
		case "potentialMergeCommit":
			q = append(q, `potentialMergeCommit{oid}`)
		case "comments":
			q = append(q, issueComments)
		case "reviewRequests":
			q = append(q, prReviewRequests)
		case "reviews":
			q = append(q, prReviews)
		case "files":
			q = append(q, prFiles)
		case "statusCheckRollup":
			q = append(q, prStatusCheckRollup)
		default:
			q = append(q, field)
		}
	}
	return strings.Join(q, ",")
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
