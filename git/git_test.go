package git

import (
	"os/exec"
	"testing"

	"github.com/cli/cli/test"
	"github.com/cli/cli/utils"
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

	teardown := utils.SetPrepareCmd(func(*exec.Cmd) utils.Runnable {
		return &test.OutputStub{}
	})
	defer teardown()

	for _, v := range cases {
		_ = utils.SetPrepareCmd(func(*exec.Cmd) utils.Runnable {
			return &test.OutputStub{[]byte(v.Output)}
		})
		ucc, _ := UncommittedChangeCount()

		if ucc != v.Expected {
			t.Errorf("got unexpected ucc value: %d for case %s", ucc, v.Label)
		}
	}
}
