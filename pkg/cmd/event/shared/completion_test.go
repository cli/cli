package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompleteEventTypes(t *testing.T) {
	const (
		review = "com.github.pull_request.review.submitted.v1\ta review was submitted"
		head   = "com.github.pull_request.head.changed.v1\tthe head branch changed, for example after a push"
		checks = "com.github.pull_request.check_suite.status.changed.v1\ta check suite on the head commit completed, such as a CI workflow"
	)

	tests := []struct {
		name       string
		toComplete string
		want       []string
	}{
		{
			name:       "matches by prefix",
			toComplete: "com.github.pull_request.re",
			want:       []string{review},
		},
		{
			name:       "matches a check suite event",
			toComplete: "com.github.pull_request.ch",
			want:       []string{checks},
		},
		{
			name:       "completes the last item and skips the chosen ones",
			toComplete: "com.github.pull_request.review.submitted.v1,com.github.pull_request.he",
			want:       []string{"com.github.pull_request.review.submitted.v1," + head},
		},
		{
			name:       "leaves out a type already chosen",
			toComplete: "com.github.pull_request.review.submitted.v1,com.github.pull_request.review",
			want:       nil,
		},
		{
			name:       "suggests nothing for an unknown type",
			toComplete: "com.github.issue",
			want:       nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CompleteEventTypes(tt.toComplete))
		})
	}
}
