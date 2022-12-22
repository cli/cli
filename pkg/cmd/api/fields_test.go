package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_parseFields(t *testing.T) {
	ios, stdin, _, _ := iostreams.Test()
	fmt.Fprint(stdin, "pasted contents")

	opts := ApiOptions{
		IO: ios,
		RawFields: []string{
			"robot=Hubot",
			"destroyer=false",
			"helper=true",
			"location=@work",
		},
		MagicFields: []string{
			"input=@-",
			"enabled=true",
			"victories=123",
		},
	}

	params, err := parseFields(&opts)
	if err != nil {
		t.Fatalf("parseFields error: %v", err)
	}

	expect := map[string]interface{}{
		"robot":     "Hubot",
		"destroyer": "false",
		"helper":    "true",
		"location":  "@work",
		"input":     "pasted contents",
		"enabled":   true,
		"victories": 123,
	}
	assert.Equal(t, expect, params)
}

func Test_parseFields_nested(t *testing.T) {
	ios, stdin, _, _ := iostreams.Test()
	fmt.Fprint(stdin, "pasted contents")

	opts := ApiOptions{
		IO: ios,
		RawFields: []string{
			"branch[name]=patch-1",
			"robots[]=Hubot",
			"robots[]=Dependabot",
			"labels[][name]=bug",
			"labels[][color]=red",
			"labels[][name]=feature",
			"labels[][color]=green",
			"empty[]",
		},
		MagicFields: []string{
			"branch[protections]=true",
			"ids[]=123",
			"ids[]=456",
		},
	}

	params, err := parseFields(&opts)
	if err != nil {
		t.Fatalf("parseFields error: %v", err)
	}

	jsonData, err := json.MarshalIndent(params, "", "\t")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, strings.TrimSuffix(heredoc.Doc(`
		{
			"branch": {
				"name": "patch-1",
				"protections": true
			},
			"empty": [],
			"ids": [
				123,
				456
			],
			"labels": [
				{
					"color": "red",
					"name": "bug"
				},
				{
					"color": "green",
					"name": "feature"
				}
			],
			"robots": [
				"Hubot",
				"Dependabot"
			]
		}
	`), "\n"), string(jsonData))
}

func Test_magicFieldValue(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gh-test")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	fmt.Fprint(f, "file contents")

	ios, _, _, _ := iostreams.Test()

	type args struct {
		v    string
		opts *ApiOptions
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			name:    "string",
			args:    args{v: "hello"},
			want:    "hello",
			wantErr: false,
		},
		{
			name:    "bool true",
			args:    args{v: "true"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "bool false",
			args:    args{v: "false"},
			want:    false,
			wantErr: false,
		},
		{
			name:    "null",
			args:    args{v: "null"},
			want:    nil,
			wantErr: false,
		},
		{
			name: "placeholder colon",
			args: args{
				v: ":owner",
				opts: &ApiOptions{
					IO: ios,
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
				},
			},
			want:    "hubot",
			wantErr: false,
		},
		{
			name: "placeholder braces",
			args: args{
				v: "{owner}",
				opts: &ApiOptions{
					IO: ios,
					BaseRepo: func() (ghrepo.Interface, error) {
						return ghrepo.New("hubot", "robot-uprising"), nil
					},
				},
			},
			want:    "hubot",
			wantErr: false,
		},
		{
			name: "file",
			args: args{
				v:    "@" + f.Name(),
				opts: &ApiOptions{IO: ios},
			},
			want:    "file contents",
			wantErr: false,
		},
		{
			name: "file error",
			args: args{
				v:    "@",
				opts: &ApiOptions{IO: ios},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := magicFieldValue(tt.args.v, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("magicFieldValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
