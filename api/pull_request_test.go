package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, err, nil)

	checks := pr.ChecksStatus()
	assert.Equal(t, checks.Total, 8)
	assert.Equal(t, checks.Pending, 3)
	assert.Equal(t, checks.Failing, 3)
	assert.Equal(t, checks.Passing, 2)
}
