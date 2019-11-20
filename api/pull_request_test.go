package api

import (
	"encoding/json"
	"testing"
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
					  "conclusion": "ACTION_REQUIRED" }
				]
			}
		}
	} }] } }
	`
	err := json.Unmarshal([]byte(payload), &pr)
	eq(t, err, nil)

	checks := pr.ChecksStatus()
	eq(t, checks.Total, 7)
	eq(t, checks.Pending, 2)
	eq(t, checks.Failing, 3)
	eq(t, checks.Passing, 2)
}
