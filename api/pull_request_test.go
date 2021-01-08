package api

import (
	"encoding/json"
	"testing"

	"github.com/cli/cli/test"
)

func TestPullRequest_ChecksStatus(t *testing.T) {
	pr := PullRequest{}
	payload := `
	{ "commits": { "nodes": [{ "commit": {
		"statusCheckRollup": {
			"contexts": {
				"nodes": [
					{ "state": "SUCCESS" },
					{ "state": "PENDING" },
					{ "state": "FAILURE" },
					{ "status": "IN_PROGRESS",
					  "conclusion": null },
					{ "status": "COMPLETED",
					  "conclusion": "SUCCESS" },
					{ "status": "COMPLETED",
					  "conclusion": "FAILURE" },
					{ "status": "COMPLETED",
					  "conclusion": "ACTION_REQUIRED" },
					{ "status": "COMPLETED",
					  "conclusion": "STALE" }
				]
			}
		}
	} }] } }
	`
	err := json.Unmarshal([]byte(payload), &pr)
	test.Eq(t, err, nil)

	checks := pr.ChecksStatus()
	test.Eq(t, checks.Total, 8)
	test.Eq(t, checks.Pending, 3)
	test.Eq(t, checks.Failing, 3)
	test.Eq(t, checks.Passing, 2)
}
