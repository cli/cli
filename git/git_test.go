package git

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/test"
)

func Test_UncommittedChangeCount(t *testing.T) {
	type c struct {
		Label    string
		Expected int
		Output   string
	}
	cases := []c{
		c{Label: "no changes", Expected: 0, Output: ""},
		c{Label: "one change", Expected: 1, Output: " M poem.txt"},
		c{Label: "untracked file", Expected: 2, Output: " M poem.txt\n?? new.txt"},
	}

	teardown := run.SetPrepareCmd(func(*exec.Cmd) run.Runnable {
		return &test.OutputStub{}
	})
	defer teardown()

	for _, v := range cases {
		_ = run.SetPrepareCmd(func(*exec.Cmd) run.Runnable {
			return &test.OutputStub{[]byte(v.Output), nil}
		})
		ucc, _ := UncommittedChangeCount()

		if ucc != v.Expected {
			t.Errorf("got unexpected ucc value: %d for case %s", ucc, v.Label)
		}
	}
}

func Test_CurrentBranch_no_commits(t *testing.T) {
	cs, teardown := test.InitCmdStubber()
	defer teardown()

	expected := "cool_branch-name"

	cs.StubError(fmt.Sprintf("fatal: your current branch '%s' does not have any commits yet", expected))

	result, err := CurrentBranch()
	if err != nil {
		t.Errorf("got unexpected error: %w", err)
	}
	if result != expected {
		t.Errorf("unexpected branch name: %s instead of %s", result, expected)
	}

}
