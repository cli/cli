package git

import (
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
			return &test.OutputStub{Out: []byte(v.Output)}
		})
		ucc, _ := UncommittedChangeCount()

		if ucc != v.Expected {
			t.Errorf("got unexpected ucc value: %d for case %s", ucc, v.Label)
		}
	}
}

func Test_CurrentBranch(t *testing.T) {
	cs, teardown := test.InitCmdStubber()
	defer teardown()

	expected := "branch-name"

	cs.Stub(expected)

	result, err := CurrentBranch()
	if err != nil {
		t.Errorf("got unexpected error: %w", err)
	}
	if len(cs.Calls) != 1 {
		t.Errorf("expected 1 git call, saw %d", len(cs.Calls))
	}
	if result != expected {
		t.Errorf("unexpected branch name: %s instead of %s", result, expected)
	}
}

func Test_CurrentBranch_detached_head(t *testing.T) {
	cs, teardown := test.InitCmdStubber()
	defer teardown()

	cs.StubError("")

	_, err := CurrentBranch()
	if err == nil {
		t.Errorf("expected an error")
	}
	expectedError := "git: not on any branch"
	if err.Error() != expectedError {
		t.Errorf("got unexpected error: %s instead of %s", err.Error(), expectedError)
	}
	if len(cs.Calls) != 1 {
		t.Errorf("expected 1 git call, saw %d", len(cs.Calls))
	}
}

func Test_CurrentBranch_unexpected_error(t *testing.T) {
	cs, teardown := test.InitCmdStubber()
	defer teardown()

	cs.StubError("lol")

	expectedError := "lol\nstub: lol"

	_, err := CurrentBranch()
	if err == nil {
		t.Errorf("expected an error")
	}
	if err.Error() != expectedError {
		t.Errorf("got unexpected error: %s instead of %s", err.Error(), expectedError)
	}
	if len(cs.Calls) != 1 {
		t.Errorf("expected 1 git call, saw %d", len(cs.Calls))
	}
}
