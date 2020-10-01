package create

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
)

func Test_Write(t *testing.T) {
	type input struct {
		in     string
		regexp *regexp.Regexp
		repl   string
	}
	type output struct {
		wantsErr bool
		out      string
		length   int
	}
	tests := []struct {
		name   string
		input  input
		output output
	}{
		{
			name: "single line input",
			input: input{
				in:     "some input line that has wrong information",
				regexp: regexp.MustCompile("wrong"),
				repl:   "right",
			},
			output: output{
				wantsErr: false,
				out:      "some input line that has right information\n",
				length:   42,
			},
		},
		{
			name: "multiple line input",
			input: input{
				in:     "multiple lines\nin this\ninput lines",
				regexp: regexp.MustCompile("lines"),
				repl:   "tests",
			},
			output: output{
				wantsErr: false,
				out:      "multiple tests\nin this\ninput tests\n",
				length:   34,
			},
		},
		{
			name: "no matches",
			input: input{
				in:     "this line has no matches",
				regexp: regexp.MustCompile("wrong"),
				repl:   "right",
			},
			output: output{
				wantsErr: false,
				out:      "this line has no matches\n",
				length:   24,
			},
		},
		{
			name: "no output",
			input: input{
				in:     "remove this whole line",
				regexp: regexp.MustCompile("^remove.*$"),
				repl:   "",
			},
			output: output{
				wantsErr: false,
				out:      "",
				length:   22,
			},
		},
		{
			name: "no input",
			input: input{
				in:     "",
				regexp: regexp.MustCompile("remove"),
				repl:   "",
			},
			output: output{
				wantsErr: false,
				out:      "",
				length:   0,
			},
		},
		{
			name: "multiple lines removed",
			input: input{
				in:     "begining line\nremove this whole line\nremove this one also\nnot this one",
				regexp: regexp.MustCompile("^remove.*$"),
				repl:   "",
			},
			output: output{
				wantsErr: false,
				out:      "begining line\nnot this one\n",
				length:   70,
			},
		},
		{
			name: "removes remote from git push output",
			input: input{
				in: heredoc.Doc(`
					output: some information
					remote: 
					remote: Create a pull request for 'regex' on GitHub by visiting: 
					remote:      https://github.com/owner/repo/pull/new/regex
					remote: 
					output: more information
			 `),
				regexp: regexp.MustCompile("^remote: (|Create a pull request for '.*' on GitHub by visiting:.*|.*https://github\\.com/.*/pull/new/.*)$"),
				repl:   "",
			},
			output: output{
				wantsErr: false,
				out:      "output: some information\noutput: more information\n",
				length:   192,
			},
		},
	}

	for _, tt := range tests {
		out := &bytes.Buffer{}
		writer := NewRegexWriter(out, tt.input.regexp, tt.input.repl)
		t.Run(tt.name, func(t *testing.T) {
			length, err := writer.Write([]byte(tt.input.in))

			if tt.output.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.out, out.String())
			assert.Equal(t, tt.output.length, length)
		})
	}
}
