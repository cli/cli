package checks

import (
	"fmt"
	"sort"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func addRow(tp *tableprinter.TablePrinter, io *iostreams.IOStreams, o check) {
	cs := io.ColorScheme()
	elapsed := ""

	if !o.StartedAt.IsZero() && !o.CompletedAt.IsZero() {
		e := o.CompletedAt.Sub(o.StartedAt)
		if e > 0 {
			elapsed = e.String()
		}
	}

	mark := "✓"
	markColor := cs.Green
	switch o.Bucket {
	case "fail":
		mark = "X"
		markColor = cs.Red
	case "pending":
		mark = "*"
		markColor = cs.Yellow
	case "skipping", "cancel":
		mark = "-"
		markColor = cs.Gray
	}

	if io.IsStdoutTTY() {
		var name string
		if o.Workflow != "" {
			name += fmt.Sprintf("%s/", o.Workflow)
		}
		name += o.Name
		if o.Event != "" {
			name += fmt.Sprintf(" (%s)", o.Event)
		}
		tp.AddField(mark, tableprinter.WithColor(markColor))
		tp.AddField(name)
		tp.AddField(o.Description)
		tp.AddField(elapsed)
		tp.AddField(o.Link)
	} else {
		tp.AddField(o.Name)
		if o.Bucket == "cancel" {
			tp.AddField("fail")
		} else {
			tp.AddField(o.Bucket)
		}
		if elapsed == "" {
			tp.AddField("0")
		} else {
			tp.AddField(elapsed)
		}
		tp.AddField(o.Link)
		tp.AddField(o.Description)
	}

	tp.EndRow()
}

func printSummary(io *iostreams.IOStreams, counts checkCounts) {
	summary := ""
	if counts.Failed+counts.Passed+counts.Skipping+counts.Pending > 0 {
		if counts.Failed > 0 {
			summary = "Some checks were not successful"
		} else if counts.Pending > 0 {
			summary = "Some checks are still pending"
		} else if counts.Canceled > 0 {
			summary = "Some checks were cancelled"
		} else {
			summary = "All checks were successful"
		}

		tallies := fmt.Sprintf("%d cancelled, %d failing, %d successful, %d skipped, and %d pending checks",
			counts.Canceled, counts.Failed, counts.Passed, counts.Skipping, counts.Pending)

		summary = fmt.Sprintf("%s\n%s", io.ColorScheme().Bold(summary), tallies)
	}

	if io.IsStdoutTTY() {
		fmt.Fprintln(io.Out, summary)
		fmt.Fprintln(io.Out)
	}
}

func printTable(io *iostreams.IOStreams, checks []check) error {
	var headers []string
	if io.IsStdoutTTY() {
		headers = []string{"", "NAME", "DESCRIPTION", "ELAPSED", "URL"}
	} else {
		headers = []string{"NAME", "STATUS", "ELAPSED", "URL", "DESCRIPTION"}
	}

	tp := tableprinter.New(io, tableprinter.WithHeader(headers...))

	sort.Slice(checks, func(i, j int) bool {
		b0 := checks[i].Bucket
		n0 := checks[i].Name
		l0 := checks[i].Link
		b1 := checks[j].Bucket
		n1 := checks[j].Name
		l1 := checks[j].Link

		if b0 == b1 {
			if n0 == n1 {
				return l0 < l1
			}
			return n0 < n1
		}

		return (b0 == "fail") || (b0 == "pending" && b1 == "success")
	})

	for _, o := range checks {
		addRow(tp, io, o)
	}

	err := tp.Render()
	if err != nil {
		return err
	}

	return nil
}
