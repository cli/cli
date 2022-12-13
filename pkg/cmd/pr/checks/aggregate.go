package checks

import (
	"fmt"
	"sort"
	"time"

	"github.com/cli/cli/v2/api"
)

type check struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Link        string    `json:"link"`
	Bucket      string    `json:"bucket"`
}

type checkCounts struct {
	Failed   int
	Passed   int
	Pending  int
	Skipping int
}

func aggregateChecks(checkContexts []api.CheckContext, requiredChecks bool) (checks []check, counts checkCounts) {
	for _, c := range eliminateDuplicates(checkContexts) {
		if requiredChecks && !c.IsRequired {
			continue
		}

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
			counts.Passed++
		case "SKIPPED", "NEUTRAL":
			item.Bucket = "skipping"
			counts.Skipping++
		case "ERROR", "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED":
			item.Bucket = "fail"
			counts.Failed++
		default: // "EXPECTED", "REQUESTED", "WAITING", "QUEUED", "PENDING", "IN_PROGRESS", "STALE"
			item.Bucket = "pending"
			counts.Pending++
		}

		checks = append(checks, item)
	}
	return
}

// eliminateDuplicates filters a set of checks to only the most recent ones if the set includes repeated runs
func eliminateDuplicates(checkContexts []api.CheckContext) []api.CheckContext {
	sort.Slice(checkContexts, func(i, j int) bool { return checkContexts[i].StartedAt.After(checkContexts[j].StartedAt) })

	mapChecks := make(map[string]struct{})
	mapContexts := make(map[string]struct{})
	unique := make([]api.CheckContext, 0, len(checkContexts))

	for _, ctx := range checkContexts {
		if ctx.Context != "" {
			if _, exists := mapContexts[ctx.Context]; exists {
				continue
			}
			mapContexts[ctx.Context] = struct{}{}
		} else {
			key := fmt.Sprintf("%s/%s", ctx.Name, ctx.CheckSuite.WorkflowRun.Workflow.Name)
			if _, exists := mapChecks[key]; exists {
				continue
			}
			mapChecks[key] = struct{}{}
		}
		unique = append(unique, ctx)
	}

	return unique
}
