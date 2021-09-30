package output

import (
	"fmt"
	"io"
)

type tabwriter struct {
	w io.Writer
}

func (j *tabwriter) SetHeader([]string) {}

func (j *tabwriter) Append(values []string) {
	var sep string
	for i, v := range values {
		if i == 1 {
			sep = "\t"
		}
		fmt.Fprintf(j.w, "%s%s", sep, v)
	}
	fmt.Fprint(j.w, "\n")
}

func (j *tabwriter) Render() {}
