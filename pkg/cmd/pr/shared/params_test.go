package shared

import "testing"

func Test_listURLWithQuery(t *testing.T) {
	type args struct {
		listURL string
		options FilterOptions
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "blank",
			args: args{
				listURL: "https://example.com/path?a=b",
				options: FilterOptions{
					Entity: "issue",
					State:  "open",
				},
			},
			want:    "https://example.com/path?a=b&q=is%3Aissue+is%3Aopen",
			wantErr: false,
		},
		{
			name: "all",
			args: args{
				listURL: "https://example.com/path",
				options: FilterOptions{
					Entity:     "issue",
					State:      "open",
					Assignee:   "bo",
					Author:     "ka",
					BaseBranch: "trunk",
					Mention:    "nu",
				},
			},
			want:    "https://example.com/path?q=is%3Aissue+is%3Aopen+assignee%3Abo+author%3Aka+base%3Atrunk+mentions%3Anu",
			wantErr: false,
		},
		{
			name: "spaces in values",
			args: args{
				listURL: "https://example.com/path",
				options: FilterOptions{
					Entity:    "pr",
					State:     "open",
					Labels:    []string{"docs", "help wanted"},
					Milestone: `Codename "What Was Missing"`,
				},
			},
			want:    "https://example.com/path?q=is%3Apr+is%3Aopen+label%3Adocs+label%3A%22help+wanted%22+milestone%3A%22Codename+%5C%22What+Was+Missing%5C%22%22",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ListURLWithQuery(tt.args.listURL, tt.args.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("listURLWithQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("listURLWithQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}
