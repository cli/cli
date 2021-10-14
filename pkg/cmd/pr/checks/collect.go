package checks

import (
	"fmt"
	"sort"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
)

type check struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Link        string    `json:"link"`
	Bucket      string    `json:"bucket"`
}

type checksMeta struct {
	Failed   int
	Passed   int
	Pending  int
	Skipping int
}

func collect(opts *ChecksOptions) ([]check, checksMeta, error) {
	checks := []check{}
	meta := checksMeta{}

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "baseRefName", "statusCheckRollup"},
	}
	pr, _, err := opts.Finder.Find(findOptions)
	if err != nil {
		return checks, meta, err
	}

	if len(pr.StatusCheckRollup.Nodes) == 0 {
		return checks, meta, fmt.Errorf("no commit found on the pull request")
	}

	rollup := pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes
	if len(rollup) == 0 {
		return checks, meta, fmt.Errorf("no checks reported on the '%s' branch", pr.BaseRefName)
	}

	checkContexts := pr.StatusCheckRollup.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes
	for _, c := range eliminateDuplicates(checkContexts) {
		state := c.State
		if state == "" {
			if c.Status == "COMPLETED" {
				state = c.Conclusion
			} else {
				state = c.Status
			}
		}

		link := c.DetailsURL
		if link == "" {
			link = c.TargetURL
		}

		name := c.Name
		if name == "" {
			name = c.Context
		}

		item := check{
			Name:        name,
			State:       state,
			StartedAt:   c.StartedAt,
			CompletedAt: c.CompletedAt,
			Link:        link,
		}
		switch state {
		case "SUCCESS":
			item.Bucket = "pass"
			meta.Passed++
		case "SKIPPED", "NEUTRAL":
			item.Bucket = "skipping"
			meta.Skipping++
		case "ERROR", "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			item.Bucket = "fail"
			meta.Failed++
		default: // "EXPECTED", "REQUESTED", "WAITING", "QUEUED", "PENDING", "IN_PROGRESS", "STALE"
			item.Bucket = "pending"
			meta.Pending++
		}

		checks = append(checks, item)
	}

	return checks, meta, nil
}

func eliminateDuplicates(checkContexts []api.CheckContext) []api.CheckContext {
	// To return the most recent check, sort in descending order by StartedAt.
	sort.Slice(checkContexts, func(i, j int) bool { return checkContexts[i].StartedAt.After(checkContexts[j].StartedAt) })

	m := make(map[string]struct{})
	unique := make([]api.CheckContext, 0, len(checkContexts))

	// Eliminate duplicates using Name or Context.
	for _, ctx := range checkContexts {
		key := ctx.Name
		if key == "" {
			key = ctx.Context
		}
		if _, ok := m[key]; ok {
			continue
		}
		unique = append(unique, ctx)
		m[key] = struct{}{}
	}

	return unique
}
