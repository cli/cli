package multigraphql

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	type args struct {
		q string
	}
	tests := []struct {
		name string
		args args
		want Query
	}{
		{
			name: "Query with variables",
			args: args{
				q: `
query($name: String!, $perPage: Int = 30) {
	a { b }
}
`,
			},
			want: Query{
				query:     "\ta { b }",
				fragments: "",
				variables: map[string]string{
					"name":    "String!",
					"perPage": "Int = 30",
				},
			},
		},
		{
			name: "Query with multi-line variables",
			args: args{
				q: `
query(
	$name: String!,
	$perPage: Int = 30,
) {
	a { b }
}
`,
			},
			want: Query{
				query:     "\ta { b }",
				fragments: "",
				variables: map[string]string{
					"name":    "String!",
					"perPage": "Int = 30",
				},
			},
		},
		{
			name: "Query with comma-less variables",
			args: args{
				q: `
query($name: String!$perPage: Int = 30,
	$user : String
	$state : [State!] = OPEN
) {
	a { b }
}
`,
			},
			want: Query{
				query:     "\ta { b }",
				fragments: "",
				variables: map[string]string{
					"name":    "String!",
					"perPage": "Int = 30",
					"user":    "String",
					"state":   "[State!] = OPEN",
				},
			},
		},
		{
			name: "Query with fragments",
			args: args{
				q: `
fragment a on A { foo }
fragment b on B { bar }
query {
	a { ...b }
}
`,
			},
			want: Query{
				query:     "\ta { ...b }",
				fragments: "\nfragment a on A { foo }\nfragment b on B { bar }\n",
				variables: map[string]string{},
			},
		},
		{
			name: "Query with no keyword",
			args: args{
				q: `
fragment b on B { bar }
{ a { ...b } }
`,
			},
			want: Query{
				query:     "a { ...b }",
				fragments: "\nfragment b on B { bar }\n",
				variables: map[string]string{},
			},
		},
		{
			name: "Malformed query",
			args: args{
				q: `a { b }`,
			},
			want: Query{
				query:     "a { b }",
				fragments: "",
				variables: map[string]string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.args.q); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}
