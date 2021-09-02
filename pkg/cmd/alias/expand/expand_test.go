package expand

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
)

func TestExpandAlias(t *testing.T) {
	findShFunc := func() (string, error) {
		return "/usr/bin/sh", nil
	}

	cfg := config.NewFromString(heredoc.Doc(`
		aliases:
		  co: pr checkout
		  il: issue list --author="$1" --label="$2"
		  ia: issue list --author="$1" --assignee="$1"
	`))

	type args struct {
		config config.Config
		argv   []string
	}
	tests := []struct {
		name         string
		args         args
		wantExpanded []string
		wantIsShell  bool
		wantErr      error
	}{
		{
			name: "no arguments",
			args: args{
				config: cfg,
				argv:   []string{},
			},
			wantExpanded: []string(nil),
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "too few arguments",
			args: args{
				config: cfg,
				argv:   []string{"gh"},
			},
			wantExpanded: []string(nil),
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "no expansion",
			args: args{
				config: cfg,
				argv:   []string{"gh", "pr", "status"},
			},
			wantExpanded: []string{"pr", "status"},
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "simple expansion",
			args: args{
				config: cfg,
				argv:   []string{"gh", "co"},
			},
			wantExpanded: []string{"pr", "checkout"},
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "adding arguments after expansion",
			args: args{
				config: cfg,
				argv:   []string{"gh", "co", "123"},
			},
			wantExpanded: []string{"pr", "checkout", "123"},
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "not enough arguments for expansion",
			args: args{
				config: cfg,
				argv:   []string{"gh", "il"},
			},
			wantExpanded: []string{},
			wantIsShell:  false,
			wantErr:      errors.New(`not enough arguments for alias: issue list --author="$1" --label="$2"`),
		},
		{
			name: "not enough arguments for expansion 2",
			args: args{
				config: cfg,
				argv:   []string{"gh", "il", "vilmibm"},
			},
			wantExpanded: []string{},
			wantIsShell:  false,
			wantErr:      errors.New(`not enough arguments for alias: issue list --author="vilmibm" --label="$2"`),
		},
		{
			name: "satisfy expansion arguments",
			args: args{
				config: cfg,
				argv:   []string{"gh", "il", "vilmibm", "help wanted"},
			},
			wantExpanded: []string{"issue", "list", "--author=vilmibm", "--label=help wanted"},
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "mixed positional and non-positional arguments",
			args: args{
				config: cfg,
				argv:   []string{"gh", "il", "vilmibm", "epic", "-R", "monalisa/testing"},
			},
			wantExpanded: []string{"issue", "list", "--author=vilmibm", "--label=epic", "-R", "monalisa/testing"},
			wantIsShell:  false,
			wantErr:      nil,
		},
		{
			name: "dollar in expansion",
			args: args{
				config: cfg,
				argv:   []string{"gh", "ia", "$coolmoney$"},
			},
			wantExpanded: []string{"issue", "list", "--author=$coolmoney$", "--assignee=$coolmoney$"},
			wantIsShell:  false,
			wantErr:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpanded, gotIsShell, err := ExpandAlias(tt.args.config, tt.args.argv, findShFunc)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error")
				}
				if tt.wantErr.Error() != err.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("got error: %v", err)
			}
			if !reflect.DeepEqual(gotExpanded, tt.wantExpanded) {
				t.Errorf("ExpandAlias() gotExpanded = %v, want %v", gotExpanded, tt.wantExpanded)
			}
			if gotIsShell != tt.wantIsShell {
				t.Errorf("ExpandAlias() gotIsShell = %v, want %v", gotIsShell, tt.wantIsShell)
			}
		})
	}
}

// cfg := `---
// aliases:
//   co: pr checkout
//   il: issue list --author="$1" --label="$2"
//   ia: issue list --author="$1" --assignee="$1"
// `
// 	initBlankContext(cfg, "OWNER/REPO", "trunk")
// 	for _, c := range []struct {
// 		Args         string
// 		ExpectedArgs []string
// 		Err          string
// 	}{
// 		{"gh co", []string{"pr", "checkout"}, ""},
// 		{"gh il", nil, `not enough arguments for alias: issue list --author="$1" --label="$2"`},
// 		{"gh il vilmibm", nil, `not enough arguments for alias: issue list --author="vilmibm" --label="$2"`},
// 		{"gh co 123", []string{"pr", "checkout", "123"}, ""},
// 		{"gh il vilmibm epic", []string{"issue", "list", `--author=vilmibm`, `--label=epic`}, ""},
// 		{"gh ia vilmibm", []string{"issue", "list", `--author=vilmibm`, `--assignee=vilmibm`}, ""},
// 		{"gh ia $coolmoney$", []string{"issue", "list", `--author=$coolmoney$`, `--assignee=$coolmoney$`}, ""},
// 		{"gh pr status", []string{"pr", "status"}, ""},
// 		{"gh il vilmibm epic -R vilmibm/testing", []string{"issue", "list", "--author=vilmibm", "--label=epic", "-R", "vilmibm/testing"}, ""},
// 		{"gh dne", []string{"dne"}, ""},
// 		{"gh", []string{}, ""},
// 		{"", []string{}, ""},
// 	} {
