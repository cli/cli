package githubsearch

import (
	"fmt"
	"strings"
)

type EntityType string
type State string
type RepoVisibility string
type SortField string
type SortDirection int

const (
	Asc SortDirection = iota
	Desc

	UpdatedAt SortField = "updated"
	CreatedAt SortField = "created"

	Issue       EntityType = "issue"
	PullRequest EntityType = "pr"

	Open   State = "open"
	Closed State = "closed"
	Merged State = "merged"

	Public  RepoVisibility = "public"
	Private RepoVisibility = "private"
)

func NewQuery() *Query {
	return &Query{}
}

type Query struct {
	repo  string
	owner string
	sort  string
	query string

	entity     string
	state      string
	baseBranch string
	headBranch string
	labels     []string
	assignee   string
	author     string
	mentions   string
	milestone  string

	language   string
	topic      string
	forkState  string
	visibility string
	isArchived *bool
}

func (q *Query) InRepository(nameWithOwner string) {
	q.repo = nameWithOwner
}

func (q *Query) OwnedBy(owner string) {
	q.owner = owner
}

func (q *Query) SortBy(field SortField, direction SortDirection) {
	var dir string
	switch direction {
	case Asc:
		dir = "asc"
	case Desc:
		dir = "desc"
	}
	q.sort = fmt.Sprintf("%s-%s", field, dir)
}

func (q *Query) AddQuery(query string) {
	q.query = query
}

func (q *Query) SetType(t EntityType) {
	q.entity = string(t)
}

func (q *Query) SetState(s State) {
	q.state = string(s)
}

func (q *Query) SetBaseBranch(name string) {
	q.baseBranch = name
}

func (q *Query) SetHeadBranch(name string) {
	q.headBranch = name
}

func (q *Query) AssignedTo(user string) {
	q.assignee = user
}

func (q *Query) AuthoredBy(user string) {
	q.author = user
}

func (q *Query) Mentions(handle string) {
	q.mentions = handle
}

func (q *Query) InMilestone(title string) {
	q.milestone = title
}

func (q *Query) AddLabel(name string) {
	q.labels = append(q.labels, name)
}

func (q *Query) SetLanguage(name string) {
	q.language = name
}

func (q *Query) SetTopic(name string) {
	q.topic = name
}

func (q *Query) SetVisibility(visibility RepoVisibility) {
	q.visibility = string(visibility)
}

func (q *Query) OnlyForks() {
	q.forkState = "only"
}

func (q *Query) IncludeForks(include bool) {
	q.forkState = fmt.Sprintf("%v", include)
}

func (q *Query) SetArchived(isArchived bool) {
	q.isArchived = &isArchived
}

func (q *Query) String() string {
	var qs string

	// context
	if q.repo != "" {
		qs += fmt.Sprintf("repo:%s ", q.repo)
	} else if q.owner != "" {
		qs += fmt.Sprintf("user:%s ", q.owner)
	}

	// type
	if q.entity != "" {
		qs += fmt.Sprintf("is:%s ", q.entity)
	}
	if q.state != "" {
		qs += fmt.Sprintf("is:%s ", q.state)
	}

	// repositories
	if q.visibility != "" {
		qs += fmt.Sprintf("is:%s ", q.visibility)
	}
	if q.language != "" {
		qs += fmt.Sprintf("language:%s ", quote(q.language))
	}
	if q.topic != "" {
		qs += fmt.Sprintf("topic:%s ", quote(q.topic))
	}
	if q.forkState != "" {
		qs += fmt.Sprintf("fork:%s ", q.forkState)
	}
	if q.isArchived != nil {
		qs += fmt.Sprintf("archived:%v ", *q.isArchived)
	}

	// issues
	if q.assignee != "" {
		qs += fmt.Sprintf("assignee:%s ", q.assignee)
	}
	for _, label := range q.labels {
		qs += fmt.Sprintf("label:%s ", quote(label))
	}
	if q.author != "" {
		qs += fmt.Sprintf("author:%s ", q.author)
	}
	if q.mentions != "" {
		qs += fmt.Sprintf("mentions:%s ", q.mentions)
	}
	if q.milestone != "" {
		qs += fmt.Sprintf("milestone:%s ", quote(q.milestone))
	}

	// pull requests
	if q.baseBranch != "" {
		qs += fmt.Sprintf("base:%s ", quote(q.baseBranch))
	}
	if q.headBranch != "" {
		qs += fmt.Sprintf("head:%s ", quote(q.headBranch))
	}

	if q.sort != "" {
		qs += fmt.Sprintf("sort:%s ", q.sort)
	}
	return strings.TrimRight(qs+q.query, " ")
}

func quote(v string) string {
	if strings.ContainsAny(v, " \"\t\r\n") {
		return fmt.Sprintf("%q", v)
	}
	return v
}
