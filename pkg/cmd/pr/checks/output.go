package checks

import (
	"fmt"
	"sort"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
)

func addRow(tp utils.TablePrinter, io *iostreams.IOStreams, o check) {
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
	case "skipping":
		mark = "-"
		markColor = cs.Gray
	}

	if io.IsStdoutTTY() {
		tp.AddField(mark, nil, markColor)
		tp.AddField(o.Name, nil, nil)
		tp.AddField(elapsed, nil, nil)
		tp.AddField(o.Link, nil, nil)
	} else {
		tp.AddField(o.Name, nil, nil)
		tp.AddField(o.Bucket, nil, nil)
		if elapsed == "" {
			tp.AddField("0", nil, nil)
		} else {
			tp.AddField(elapsed, nil, nil)
		}
		tp.AddField(o.Link, nil, nil)
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
		} else {
			summary = "All checks were successful"
		}

		tallies := fmt.Sprintf("%d failing, %d successful, %d skipped, and %d pending checks",
			counts.Failed, counts.Passed, counts.Skipping, counts.Pending)

		summary = fmt.Sprintf("%s\n%s", io.ColorScheme().Bold(summary), tallies)
	}

	if io.IsStdoutTTY() {
		fmt.Fprintln(io.Out, summary)
		fmt.Fprintln(io.Out)
	}
}

func printTable(io *iostreams.IOStreams, checks []check) error {
	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	tp := utils.NewTablePrinter(io)

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
