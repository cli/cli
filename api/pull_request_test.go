package api

import (
	"encoding/json"
	"testing"
)

func TestPullRequest_ChecksStatus(t *testing.T) {
	pr := PullRequestComplex{}
	payload := `
	{ "commits": { "nodes": [{ "commit": {
		"statusCheckRollup": {
			"contexts": {
				"nodes": [
					{ "StatusContext": {"state": "SUCCESS"} },
					{ "StatusContext": {"state": "PENDING"} },
					{ "StatusContext": {"state": "FAILURE"} },
					{ "CheckRun": { "status": "IN_PROGRESS",
					  "conclusion": null }},
					{ "CheckRun": { "status": "COMPLETED",
					  "conclusion": "SUCCESS" }},
					{ "CheckRun": { "status": "COMPLETED",
					  "conclusion": "FAILURE" }},
					{ "CheckRun": { "status": "COMPLETED",
					  "conclusion": "ACTION_REQUIRED" }},
					{ "CheckRun": { "status": "COMPLETED",
					  "conclusion": "STALE" }}
				]
			}
		}
	} }] } }
	`
	err := json.Unmarshal([]byte(payload), &pr)
	eq(t, err, nil)

	checks := pr.ChecksStatus()
	eq(t, checks.Total, 8)
	eq(t, checks.Pending, 3)
	eq(t, checks.Failing, 3)
	eq(t, checks.Passing, 2)
}
