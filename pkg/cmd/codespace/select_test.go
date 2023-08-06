package codespace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

const CODESPACE_NAME = "monalisa-cli-cli-abcdef"

func TestApp_Select(t *testing.T) {
	tests := []struct {
		name             string
		arg              string
		wantErr          bool
		outputToFile     bool
		wantStdout       string
		wantStderr       string
		wantFileContents string
	}{
		{
			name:       "Select a codespace",
			arg:        CODESPACE_NAME,
			wantErr:    false,
			wantStdout: fmt.Sprintf("%s\n", CODESPACE_NAME),
		},
		{
			name:    "Select a codespace error",
			arg:     "non-existent-codespace-name",
			wantErr: true,
		},
		{
			name:             "Select a codespace",
			arg:              CODESPACE_NAME,
			wantErr:          false,
			wantFileContents: CODESPACE_NAME,
			outputToFile:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			a := NewApp(ios, nil, testSelectApiMock(), nil, nil)

			opts := selectOptions{}

			if tt.outputToFile {
				file, err := os.CreateTemp("", "codespace-selection-test")
				if err != nil {
					t.Fatal(err)
				}

				defer os.Remove(file.Name())

				opts = selectOptions{filePath: file.Name()}
			}

			opts.selector = &CodespaceSelector{api: a.apiClient, codespaceName: tt.arg}

			if err := a.Select(context.Background(), opts); (err != nil) != tt.wantErr {
				t.Errorf("App.Select() error = %v, wantErr %v", err, tt.wantErr)
			}

			if out := stdout.String(); out != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", out, tt.wantStdout)
			}
			if out := sortLines(stderr.String()); out != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", out, tt.wantStderr)
			}

			if tt.wantFileContents != "" {
				if opts.filePath == "" {
					t.Errorf("wantFileContents is set but opts.filePath is not")
				}

				dat, err := os.ReadFile(opts.filePath)
				if err != nil {
					t.Fatal(err)
				}

				if string(dat) != tt.wantFileContents {
					t.Errorf("file contents = %q, want %q", string(dat), CODESPACE_NAME)
				}
			}
		})
	}
}

func testSelectApiMock() *apiClientMock {
	testingCodespace := &api.Codespace{
		Name: CODESPACE_NAME,
	}
	return &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, includeConnection bool) (*api.Codespace, error) {
			if name == CODESPACE_NAME {
				return testingCodespace, nil
			}

			return nil, errors.New("cannot find codespace")
		},
	}
}
