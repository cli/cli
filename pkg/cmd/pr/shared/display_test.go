package shared

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_listHeader(t *testing.T) {
	type args struct {
		repoName        string
		itemName        string
		matchCount      int
		totalMatchCount int
		hasFilters      bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "one result",
			args: args{
				repoName:        "REPO",
				itemName:        "genie",
				matchCount:      1,
				totalMatchCount: 23,
				hasFilters:      false,
			},
			want: "Showing 1 of 23 open genies in REPO",
		},
		{
			name: "one result after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "tiny cup",
				matchCount:      1,
				totalMatchCount: 23,
				hasFilters:      true,
			},
			want: "Showing 1 of 23 tiny cups in REPO that match your search",
		},
		{
			name: "one result in total",
			args: args{
				repoName:        "REPO",
				itemName:        "chip",
				matchCount:      1,
				totalMatchCount: 1,
				hasFilters:      false,
			},
			want: "Showing 1 of 1 open chip in REPO",
		},
		{
			name: "one result in total after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "spicy noodle",
				matchCount:      1,
				totalMatchCount: 1,
				hasFilters:      true,
			},
			want: "Showing 1 of 1 spicy noodle in REPO that matches your search",
		},
		{
			name: "multiple results",
			args: args{
				repoName:        "REPO",
				itemName:        "plant",
				matchCount:      4,
				totalMatchCount: 23,
				hasFilters:      false,
			},
			want: "Showing 4 of 23 open plants in REPO",
		},
		{
			name: "multiple results after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "boomerang",
				matchCount:      4,
				totalMatchCount: 23,
				hasFilters:      true,
			},
			want: "Showing 4 of 23 boomerangs in REPO that match your search",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ListHeader(tt.args.repoName, tt.args.itemName, tt.args.matchCount, tt.args.totalMatchCount, tt.args.hasFilters); got != tt.want {
				t.Errorf("listHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrCheckStatusSummaryWithColor(t *testing.T) {
	testCases := []struct {
		Name string
		args api.PullRequestChecksStatus
		want string
	}{
		{
			Name: "No Checks",
			args: api.PullRequestChecksStatus{
				Total:   0,
				Failing: 0,
				Passing: 0,
				Pending: 0,
			},
			want: "No checks",
		},
		{
			Name: "All Passing",
			args: api.PullRequestChecksStatus{
				Total:   3,
				Failing: 0,
				Passing: 3,
				Pending: 0,
			},
			want: "✓ Checks passing",
		},
		{
			Name: "Some pending",
			args: api.PullRequestChecksStatus{
				Total:   3,
				Failing: 0,
				Passing: 1,
				Pending: 2,
			},
			want: "- Checks pending",
		},
		{
			Name: "Sll failing",
			args: api.PullRequestChecksStatus{
				Total:   3,
				Failing: 3,
				Passing: 0,
				Pending: 0,
			},
			want: "× All checks failing",
		},
		{
			Name: "Some failing",
			args: api.PullRequestChecksStatus{
				Total:   3,
				Failing: 2,
				Passing: 1,
				Pending: 0,
			},
			want: "× 2/3 checks failing",
		},
	}

	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetAlternateScreenBufferEnabled(true)
	cs := ios.ColorScheme()

	for _, testCase := range testCases {
		out := PrCheckStatusSummaryWithColor(cs, testCase.args)
		assert.Equal(t, testCase.want, out)
	}

}
