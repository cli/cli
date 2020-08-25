package shared

import "testing"

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
			name: "no results",
			args: args{
				repoName:        "REPO",
				itemName:        "table",
				matchCount:      0,
				totalMatchCount: 0,
				hasFilters:      false,
			},
			want: "There are no open tables in REPO",
		},
		{
			name: "no matches after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "Luftballon",
				matchCount:      0,
				totalMatchCount: 0,
				hasFilters:      true,
			},
			want: "No Luftballons match your search in REPO",
		},
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
