package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
)

func Test_apiRun(t *testing.T) {
	tests := []struct {
		name         string
		httpResponse *http.Response
		err          error
		stdout       string
		stderr       string
	}{
		{
			name: "success",
			httpResponse: &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(`bam!`)),
			},
			err:    nil,
			stdout: `bam!`,
			stderr: ``,
		},
		{
			name: "failure",
			httpResponse: &http.Response{
				StatusCode: 502,
				Body:       ioutil.NopCloser(bytes.NewBufferString(`gateway timeout`)),
			},
			err:    cmdutil.SilentError,
			stdout: `gateway timeout`,
			stderr: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()

			opts := ApiOptions{
				IO: io,
				HttpClient: func() (*http.Client, error) {
					var tr roundTripper = func(req *http.Request) (*http.Response, error) {
						resp := tt.httpResponse
						resp.Request = req
						return resp, nil
					}
					return &http.Client{Transport: tr}, nil
				},

				RawFields:   []string{},
				MagicFields: []string{},
			}

			err := apiRun(&opts)
			if err != tt.err {
				t.Errorf("expected error %v, got %v", tt.err, err)
			}

			if stdout.String() != tt.stdout {
				t.Errorf("expected output %q, got %q", tt.stdout, stdout.String())
			}
			if stderr.String() != tt.stderr {
				t.Errorf("expected error output %q, got %q", tt.stderr, stderr.String())
			}
		})
	}
}

func Test_parseFields(t *testing.T) {
	io, stdin, _, _ := iostreams.Test()
	fmt.Fprint(stdin, "pasted contents")

	opts := ApiOptions{
		IO: io,
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
		"input":     []byte("pasted contents"),
		"enabled":   true,
		"victories": 123,
	}
	if !reflect.DeepEqual(params, expect) {
		t.Errorf("expected %v, got %v", expect, params)
	}
}
