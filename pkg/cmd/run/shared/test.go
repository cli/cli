package shared

import (
	"fmt"
	"time"
)

// Test data for use in the various run and job tests
func created() time.Time {
	created, _ := time.Parse("2006-01-02 15:04:05", "2021-02-23 04:51:00")
	return created
}

func updated() time.Time {
	updated, _ := time.Parse("2006-01-02 15:04:05", "2021-02-23 04:55:34")
	return updated
}

func TestRun(name string, id int, s Status, c Conclusion) Run {
	return Run{
		Name:       name,
		ID:         id,
		CreatedAt:  created(),
		UpdatedAt:  updated(),
		Status:     s,
		Conclusion: c,
		Event:      "push",
		HeadBranch: "trunk",
		JobsURL:    fmt.Sprintf("/runs/%d/jobs", id),
		HeadCommit: Commit{
			Message: "cool commit",
		},
		HeadSha: "1234567890",
		URL:     fmt.Sprintf("runs/%d", id),
		HeadRepository: Repo{
			Owner: struct{ Login string }{Login: "OWNER"},
			Name:  "REPO",
		},
	}
}

var SuccessfulRun Run = TestRun("successful", 3, Completed, Success)
var FailedRun Run = TestRun("failed", 1234, Completed, Failure)

var TestRuns []Run = []Run{
	TestRun("timed out", 1, Completed, TimedOut),
	TestRun("in progress", 2, InProgress, ""),
	SuccessfulRun,
	TestRun("cancelled", 4, Completed, Cancelled),
	FailedRun,
	TestRun("neutral", 6, Completed, Neutral),
	TestRun("skipped", 7, Completed, Skipped),
	TestRun("requested", 8, Requested, ""),
	TestRun("queued", 9, Queued, ""),
	TestRun("stale", 10, Completed, Stale),
}

var SuccessfulJob Job = Job{
	ID:          10,
	Status:      Completed,
	Conclusion:  Success,
	Name:        "cool job",
	StartedAt:   created(),
	CompletedAt: updated(),
	URL:         "jobs/10",
	Steps: []Step{
		{
			Name:       "fob the barz",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "barz the fob",
			Status:     Completed,
			Conclusion: Success,
			Number:     2,
		},
	},
}

var FailedJob Job = Job{
	ID:          20,
	Status:      Completed,
	Conclusion:  Failure,
	Name:        "sad job",
	StartedAt:   created(),
	CompletedAt: updated(),
	URL:         "jobs/20",
	Steps: []Step{
		{
			Name:       "barf the quux",
			Status:     Completed,
			Conclusion: Success,
			Number:     1,
		},
		{
			Name:       "quux the barf",
			Status:     Completed,
			Conclusion: Failure,
			Number:     2,
		},
	},
}

var FailedJobAnnotations []Annotation = []Annotation{
	{
		JobName:   "sad job",
		Message:   "the job is sad",
		Path:      "blaze.py",
		Level:     "failure",
		StartLine: 420,
	},
}
