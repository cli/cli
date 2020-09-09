package text

import "testing"

func Test_Indent(t *testing.T) {
	type args struct {
		s      string
		indent string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "empty",
			args: args{
				s:      "",
				indent: "--",
			},
			want: "",
		},
		{
			name: "blank",
			args: args{
				s:      "\n",
				indent: "--",
			},
			want: "\n",
		},
		{
			name: "indent",
			args: args{
				s:      "one\ntwo\nthree",
				indent: "--",
			},
			want: "--one\n--two\n--three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Indent(tt.args.s, tt.args.indent); got != tt.want {
				t.Errorf("indent() = %q, want %q", got, tt.want)
			}
		})
	}
}
