package shared

import (
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderJobs(t *testing.T) {
	startedAt, err := time.Parse(time.RFC3339, "2009-03-19T00:00:00Z")
	require.NoError(t, err)
	completedAt, err := time.Parse(time.RFC3339, "2009-03-19T00:01:00Z")
	require.NoError(t, err)

	tests := []struct {
		name        string
		jobs        []Job
		wantVerbose string
		wantNormal  string
		wantCompact string
	}{
		{
			name: "nil jobs",
			jobs: nil,
		},
		{
			name: "empty jobs",
			jobs: []Job{},
		},
		{
			// This is not a real-world case, but nevertheless the code should
			// be able to handle that without error/panic.
			name: "in-progress job without steps",
			jobs: []Job{
				{
					Name:      "foo",
					ID:        999,
					StartedAt: startedAt,
					Status:    InProgress,
				},
			},
			wantCompact: heredoc.Doc(`
				* foo (ID 999)`),
			wantNormal: heredoc.Doc(`
				* foo (ID 999)`),
			wantVerbose: heredoc.Doc(`
				* foo (ID 999)`),
		},
		{
			// This is not a real-world case, but nevertheless the code should
			// be able to handle that without error/panic.
			name: "successful job without steps",
			jobs: []Job{
				{
					Name:        "foo",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Success,
				},
			},
			wantCompact: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)`),
			wantNormal: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)`),
			wantVerbose: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)`),
		},
		{
			// This is not a real-world case, but nevertheless the code should
			// be able to handle that without error/panic.
			name: "failed job without steps",
			jobs: []Job{
				{
					Name:        "foo",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Failure,
				},
			},
			wantCompact: heredoc.Doc(`
				X foo in 1m0s (ID 999)`),
			wantNormal: heredoc.Doc(`
				X foo in 1m0s (ID 999)`),
			wantVerbose: heredoc.Doc(`
				X foo in 1m0s (ID 999)`),
		},
		{
			name: "in-progress job with various step status values",
			jobs: []Job{
				{
					Name:      "foo",
					ID:        999,
					StartedAt: startedAt,
					Status:    InProgress,
					Steps: []Step{
						{
							Name:        "passed",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:       "skipped",
							Status:     Completed,
							Conclusion: Skipped,
							Number:     2,
						},
						{
							Name:        "failed 1",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Failure,
							Number:      3,
						},
						{
							Name:        "failed 2",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Failure,
							Number:      4,
						},
						{
							Name:      "in-progress",
							StartedAt: startedAt,
							Status:    InProgress,
							Number:    5,
						},
						{
							Name:   "pending",
							Status: Pending,
							Number: 6,
						},
					},
				},
			},
			wantCompact: heredoc.Doc(`
				* foo (ID 999)
				  X failed 1
				  X failed 2
				  * in-progress`),
			wantNormal: heredoc.Doc(`
				* foo (ID 999)`),
			wantVerbose: heredoc.Doc(`
				* foo (ID 999)
				  ✓ passed
				  - skipped
				  X failed 1
				  X failed 2
				  * in-progress
				  * pending`),
		},
		{
			// As of my observations (babakks) when there is a failed step, the
			// job run is marked as failed. In other words, a successful job run
			// cannot have any failed steps. That's why there's no failed steps
			// in this test case.
			name: "successful job with various step status values",
			jobs: []Job{
				{
					Name:        "foo",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Success,
					Steps: []Step{
						{
							Name:        "passed 1",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:       "skipped",
							Status:     Completed,
							Conclusion: Skipped,
							Number:     2,
						},
						{
							Name:        "passed 2",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      3,
						},
					},
				},
			},
			wantCompact: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)`),
			wantNormal: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)`),
			wantVerbose: heredoc.Doc(`
				✓ foo in 1m0s (ID 999)
				  ✓ passed 1
				  - skipped
				  ✓ passed 2`),
		},
		{
			name: "failed job with various step status values",
			jobs: []Job{
				{
					Name:        "foo",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Failure,
					Steps: []Step{
						{
							Name:        "passed 1",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:       "skipped",
							Status:     Completed,
							Conclusion: Skipped,
							Number:     2,
						},
						{
							Name:        "failed 1",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Failure,
							Number:      3,
						},
						{
							Name:        "failed 2",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Failure,
							Number:      4,
						},
						{
							Name:        "passed 2",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      5,
						},
					},
				},
			},
			wantCompact: heredoc.Doc(`
				X foo in 1m0s (ID 999)
				  X failed 1
				  X failed 2`),
			wantNormal: heredoc.Doc(`
				X foo in 1m0s (ID 999)
				  ✓ passed 1
				  - skipped
				  X failed 1
				  X failed 2
				  ✓ passed 2`),
			wantVerbose: heredoc.Doc(`
				X foo in 1m0s (ID 999)
				  ✓ passed 1
				  - skipped
				  X failed 1
				  X failed 2
				  ✓ passed 2`),
		},
		{
			name: "multiple jobs",
			jobs: []Job{
				{
					Name:      "in-progress",
					ID:        999,
					StartedAt: startedAt,
					Status:    InProgress,
					Steps: []Step{
						{
							Name:        "passed",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:      "in-progress",
							StartedAt: startedAt,
							Status:    InProgress,
							Number:    2,
						},
					},
				},
				{
					Name:        "successful",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Success,
					Steps: []Step{
						{
							Name:        "passed",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:       "skipped",
							Status:     Completed,
							Conclusion: Skipped,
							Number:     2,
						},
					},
				},
				{
					Name:        "failed",
					ID:          999,
					StartedAt:   startedAt,
					CompletedAt: completedAt,
					Status:      Completed,
					Conclusion:  Failure,
					Steps: []Step{
						{
							Name:        "passed",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Success,
							Number:      1,
						},
						{
							Name:        "failed",
							StartedAt:   startedAt,
							CompletedAt: completedAt,
							Status:      Completed,
							Conclusion:  Failure,
							Number:      2,
						},
					},
				},
			},
			wantCompact: heredoc.Doc(`
				* in-progress (ID 999)
				  * in-progress
				✓ successful in 1m0s (ID 999)
				X failed in 1m0s (ID 999)
				  X failed`),
			wantNormal: heredoc.Doc(`
				* in-progress (ID 999)
				✓ successful in 1m0s (ID 999)
				X failed in 1m0s (ID 999)
				  ✓ passed
				  X failed`),
			wantVerbose: heredoc.Doc(`
				* in-progress (ID 999)
				  ✓ passed
				  * in-progress
				✓ successful in 1m0s (ID 999)
				  ✓ passed
				  - skipped
				X failed in 1m0s (ID 999)
				  ✓ passed
				  X failed`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCompact := RenderJobsCompact(&iostreams.ColorScheme{}, tt.jobs)
			assert.Equal(t, tt.wantCompact, gotCompact, "unexpected compact mode output")

			gotNormal := RenderJobs(&iostreams.ColorScheme{}, tt.jobs, false)
			assert.Equal(t, tt.wantNormal, gotNormal, "unexpected normal mode output")

			gotVerbose := RenderJobs(&iostreams.ColorScheme{}, tt.jobs, true)
			assert.Equal(t, tt.wantVerbose, gotVerbose, "unexpected verbose mode output")
		})
	}
}
