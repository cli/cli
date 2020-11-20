package shared

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/stretchr/testify/assert"
)

func Test_PreserveInput(t *testing.T) {
	tests := []struct {
		name             string
		state            *IssueMetadataState
		err              bool
		wantErrLines     []string
		wantPreservation bool
	}{
		{
			name: "err, no changes to state",
			err:  true,
		},
		{
			name: "no err, no changes to state",
			err:  false,
		},
		{
			name: "no err, changes to state",
			state: &IssueMetadataState{
				dirty: true,
			},
		},
		{
			name: "err, title/body input received",
			state: &IssueMetadataState{
				dirty:     true,
				Title:     "almost a",
				Body:      "jill sandwich",
				Reviewers: []string{"barry", "chris"},
				Labels:    []string{"sandwich"},
			},
			wantErrLines: []string{
				`X operation failed. input saved to:.*testfile.*`,
				`resubmit with: gh issue create -j@.*testfile.*`,
			},
			err:              true,
			wantPreservation: true,
		},
		{
			name: "err, metadata received",
			state: &IssueMetadataState{
				Reviewers: []string{"barry", "chris"},
				Labels:    []string{"sandwich"},
			},
			wantErrLines: []string{
				`X operation failed. input saved to:.*testfile.*`,
				`resubmit with: gh issue create -j@.*testfile.*`,
			},
			err:              true,
			wantPreservation: true,
		},
		{
			name: "err, dirty, pull request",
			state: &IssueMetadataState{
				dirty: true,
				Title: "a pull request",
				Type:  PRMetadata,
			},
			wantErrLines: []string{
				`X operation failed. input saved to:.*testfile.*`,
				`resubmit with: gh pr create -j@.*testfile.*`,
			},
			err:              true,
			wantPreservation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state == nil {
				tt.state = &IssueMetadataState{}
			}

			io, _, _, errOut := iostreams.Test()

			tfPath, tf, tferr := tmpfile()
			assert.NoError(t, tferr)
			defer os.Remove(tfPath)

			io.TempFileOverride = tf

			var err error
			if tt.err {
				err = errors.New("error during creation")
			}

			PreserveInput(io, tt.state, &err)()

			tf.Seek(0, 0)

			data, err := ioutil.ReadAll(tf)
			assert.NoError(t, err)

			if tt.wantPreservation {
				test.ExpectLines(t, errOut.String(), tt.wantErrLines...)
				preserved := &IssueMetadataState{}
				assert.NoError(t, json.Unmarshal(data, preserved))
				preserved.dirty = tt.state.dirty
				assert.Equal(t, preserved, tt.state)
			} else {
				assert.Equal(t, errOut.String(), "")
				assert.Equal(t, string(data), "")
			}
		})
	}
}

func tmpfile() (string, *os.File, error) {
	dir := os.TempDir()
	tmpfile, err := ioutil.TempFile(dir, "testfile*")
	if err != nil {
		return "", nil, err
	}

	return filepath.Join(dir, tmpfile.Name()), tmpfile, nil
}
