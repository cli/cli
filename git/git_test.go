package git

import (
	"reflect"
	"testing"

	"github.com/cli/cli/internal/run"
)

func Test_UncommittedChangeCount(t *testing.T) {
	type c struct {
		Label    string
		Expected int
		Output   string
	}
	cases := []c{
		{Label: "no changes", Expected: 0, Output: ""},
		{Label: "one change", Expected: 1, Output: " M poem.txt"},
		{Label: "untracked file", Expected: 2, Output: " M poem.txt\n?? new.txt"},
	}

	for _, v := range cases {
		t.Run(v.Label, func(t *testing.T) {
			cs, restore := run.Stub()
			defer restore(t)
			cs.Register(`git status --porcelain`, 0, v.Output)

			ucc, _ := UncommittedChangeCount()
			if ucc != v.Expected {
				t.Errorf("UncommittedChangeCount() = %d, expected %d", ucc, v.Expected)
			}
		})
	}
}

func Test_CurrentBranch(t *testing.T) {
	type c struct {
		Stub     string
		Expected string
	}
	cases := []c{
		{
			Stub:     "branch-name\n",
			Expected: "branch-name",
		},
		{
			Stub:     "refs/heads/branch-name\n",
			Expected: "branch-name",
		},
		{
			Stub:     "refs/heads/branch\u00A0with\u00A0non\u00A0breaking\u00A0space\n",
			Expected: "branch\u00A0with\u00A0non\u00A0breaking\u00A0space",
		},
	}

	for _, v := range cases {
		cs, teardown := run.Stub()
		cs.Register(`git symbolic-ref --quiet HEAD`, 0, v.Stub)

		result, err := CurrentBranch()
		if err != nil {
			t.Errorf("got unexpected error: %w", err)
		}
		if result != v.Expected {
			t.Errorf("unexpected branch name: %s instead of %s", result, v.Expected)
		}
		teardown(t)
	}
}

func Test_CurrentBranch_detached_head(t *testing.T) {
	cs, teardown := run.Stub()
	defer teardown(t)
	cs.Register(`git symbolic-ref --quiet HEAD`, 1, "")

	_, err := CurrentBranch()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrNotOnAnyBranch {
		t.Errorf("got unexpected error: %s instead of %s", err, ErrNotOnAnyBranch)
	}
}

func TestParseExtraCloneArgs(t *testing.T) {
	type Wanted struct {
		args []string
		dir  string
	}
	tests := []struct {
		name string
		args []string
		want Wanted
	}{
		{
			name: "args and target",
			args: []string{"target_directory", "-o", "upstream", "--depth", "1"},
			want: Wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "target_directory",
			},
		},
		{
			name: "only args",
			args: []string{"-o", "upstream", "--depth", "1"},
			want: Wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "",
			},
		},
		{
			name: "only target",
			args: []string{"target_directory"},
			want: Wanted{
				args: []string{},
				dir:  "target_directory",
			},
		},
		{
			name: "no args",
			args: []string{},
			want: Wanted{
				args: []string{},
				dir:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, dir := parseCloneArgs(tt.args)
			got := Wanted{
				args: args,
				dir:  dir,
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestAddUpstreamRemote(t *testing.T) {
	tests := []struct {
		name        string
		upstreamURL string
		cloneDir    string
		branches    []string
		want        string
	}{
		{
			name:        "fetch all",
			upstreamURL: "URL",
			cloneDir:    "DIRECTORY",
			branches:    []string{},
			want:        "git -C DIRECTORY remote add -f upstream URL",
		},
		{
			name:        "fetch specific branches only",
			upstreamURL: "URL",
			cloneDir:    "DIRECTORY",
			branches:    []string{"master", "dev"},
			want:        "git -C DIRECTORY remote add -t master -t dev -f upstream URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(tt.want, 0, "")

			err := AddUpstreamRemote(tt.upstreamURL, tt.cloneDir, tt.branches)
			if err != nil {
				t.Fatalf("error running command `git remote add -f`: %v", err)
			}
		})
	}
}
