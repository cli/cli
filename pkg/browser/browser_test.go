package browser

import (
	"reflect"
	"testing"
)

func TestForOS(t *testing.T) {
	type args struct {
		goos string
		url  string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "macOS",
			args: args{
				goos: "darwin",
				url:  "https://example.com/path?a=1&b=2",
			},
			want: []string{"open", "https://example.com/path?a=1&b=2"},
		},
		{
			name: "Linux",
			args: args{
				goos: "linux",
				url:  "https://example.com/path?a=1&b=2",
			},
			want: []string{"xdg-open", "https://example.com/path?a=1&b=2"},
		},
		{
			name: "Windows",
			args: args{
				goos: "windows",
				url:  "https://example.com/path?a=1&b=2&c=3",
			},
			want: []string{"cmd", "/c", "start", "https://example.com/path?a=1^&b=2^&c=3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cmd := ForOS(tt.args.goos, tt.args.url); !reflect.DeepEqual(cmd.Args, tt.want) {
				t.Errorf("ForOS() = %v, want %v", cmd.Args, tt.want)
			}
		})
	}
}
