package text

import "testing"

func TestTruncate(t *testing.T) {
	type args struct {
		max int
		s   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Japanese",
			args: args{
				max: 11,
				s:   "テストテストテストテスト",
			},
			want: "テストテ...",
		},
		{
			name: "Japanese filled",
			args: args{
				max: 11,
				s:   "aテストテストテストテスト",
			},
			want: "aテスト... ",
		},
		{
			name: "Chinese",
			args: args{
				max: 11,
				s:   "幫新舉報違章工廠新增編號",
			},
			want: "幫新舉報...",
		},
		{
			name: "Chinese filled",
			args: args{
				max: 11,
				s:   "a幫新舉報違章工廠新增編號",
			},
			want: "a幫新舉... ",
		},
		{
			name: "Korean",
			args: args{
				max: 11,
				s:   "프로젝트 내의",
			},
			want: "프로젝트...",
		},
		{
			name: "Korean filled",
			args: args{
				max: 11,
				s:   "a프로젝트 내의",
			},
			want: "a프로젝... ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Truncate(tt.args.max, tt.args.s); got != tt.want {
				t.Errorf("Truncate() = %q, want %q", got, tt.want)
			}
		})
	}
}
