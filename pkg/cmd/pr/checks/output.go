package checks

import (
	"fmt"
	"sort"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
)

func addRow(tp utils.TablePrinter, isTerminal bool, cs *iostreams.ColorScheme, o check) {
	elapsed := ""
	zeroTime := time.Time{}

	if o.StartedAt != zeroTime && o.CompletedAt != zeroTime {
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

	if isTerminal {
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

func printTable(isTerminal bool, cs *iostreams.ColorScheme, meta checksMeta, checks []check, opts *ChecksOptions) error {
	summary := ""
	if meta.Failed+meta.Passed+meta.Skipping+meta.Pending > 0 {
		if meta.Failed > 0 {
			summary = "Some checks were not successful"
		} else if meta.Pending > 0 {
			summary = "Some checks are still pending"
		} else {
			summary = "All checks were successful"
		}

		tallies := fmt.Sprintf("%d failing, %d successful, %d skipped, and %d pending checks",
			meta.Failed, meta.Passed, meta.Skipping, meta.Pending)

		summary = fmt.Sprintf("%s\n%s", cs.Bold(summary), tallies)
	}

	if isTerminal {
		fmt.Fprintln(opts.IO.Out, summary)
		fmt.Fprintln(opts.IO.Out)
	}

	tp := utils.NewTablePrinter(opts.IO)

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
		addRow(tp, isTerminal, cs, o)
	}

	err := tp.Render()
	if err != nil {
		return err
	}

	return nil
}
