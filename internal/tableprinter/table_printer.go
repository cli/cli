package tableprinter

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type fieldOption func(*tableField)

type tableField struct {
	text         string
	truncateFunc func(int, string) string
	paddingFunc  func(int, string) string
	colorFunc    func(string) string
}

type TablePrinter struct {
	out        io.Writer
	isTTY      bool
	maxWidth   int
	hasHeaders bool
	rows       [][]tableField
	currentCol int
}

func (t *TablePrinter) IsTTY() bool {
	return t.isTTY
}

func (tp *TablePrinter) AddTimeField(now, value time.Time, color func(string) string) {
	var field string
	if tp.isTTY {
		field = text.FuzzyAgo(now, value)
	} else {
		field = value.Format(time.RFC3339)
	}
	tp.AddField(field, WithColor(color))
}

func WithTruncate(fn func(int, string) string) fieldOption {
	return func(field *tableField) {
		field.truncateFunc = fn
	}
}

func WithPadding(fn func(int, string) string) fieldOption {
	return func(field *tableField) {
		field.paddingFunc = fn
	}
}

func WithColor(fn func(string) string) fieldOption {
	return func(field *tableField) {
		field.colorFunc = fn
	}
}

type headerOption struct {
	columns []string
}

func New(ios *iostreams.IOStreams, headers headerOption) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}
	return NewWithWriter(ios.Out, isTTY, maxWidth, ios.ColorScheme(), headers)
}

func NewWithWriter(writer io.Writer, isTTY bool, maxWidth int, cs *iostreams.ColorScheme, headers headerOption) *TablePrinter {
	tp := &TablePrinter{
		out:      writer,
		isTTY:    isTTY,
		maxWidth: maxWidth,
	}
	if isTTY && len(headers.columns) > 0 {
		upperCasedHeaders := make([]string, len(headers.columns))
		for i := range headers.columns {
			upperCasedHeaders[i] = strings.ToUpper(headers.columns[i])
		}
		var padding func(int, string) string
		if cs.Enabled {
			padding = text.PadRight
		}
		tp.AddHeader(upperCasedHeaders, WithPadding(padding), WithColor(cs.TableHeader))
	}
	return tp
}

func WithHeader(columns ...string) headerOption {
	if len(columns) == 0 {
		panic("must define header columns")
	}
	return headerOption{columns: columns}
}

var NoHeader = headerOption{}

func (t *TablePrinter) AddHeader(columns []string, options ...fieldOption) {
	if t.hasHeaders {
		return
	}
	t.hasHeaders = true
	for _, column := range columns {
		t.AddField(column, options...)
	}
	t.EndRow()
}

func (t *TablePrinter) AddField(value string, options ...fieldOption) {
	if t.isTTY {
		if t.rows == nil {
			t.rows = make([][]tableField, 1)
		}
		row := len(t.rows) - 1
		field := tableField{text: value, truncateFunc: text.Truncate}
		for _, option := range options {
			option(&field)
		}
		t.rows[row] = append(t.rows[row], field)
		return
	}
	if t.currentCol > 0 {
		fmt.Fprint(t.out, "\t")
	}
	fmt.Fprint(t.out, value)
	t.currentCol++
}

func (t *TablePrinter) EndRow() {
	if t.isTTY {
		t.rows = append(t.rows, []tableField{})
		return
	}
	fmt.Fprint(t.out, "\n")
	t.currentCol = 0
}

func (t *TablePrinter) Render() error {
	if !t.isTTY || len(t.rows) == 0 {
		return nil
	}
	const delimiter = "  "
	numColumns := len(t.rows[0])
	columnWidths := t.calculateColumnWidths(len(delimiter))
	for _, row := range t.rows {
		for column, field := range row {
			if column > 0 {
				if _, err := fmt.Fprint(t.out, delimiter); err != nil {
					return err
				}
			}
			value := field.text
			if field.truncateFunc != nil {
				value = field.truncateFunc(columnWidths[column], value)
			}
			if field.paddingFunc != nil {
				value = field.paddingFunc(columnWidths[column], value)
			} else if column < numColumns-1 {
				value = text.PadRight(columnWidths[column], value)
			}
			if field.colorFunc != nil {
				value = field.colorFunc(value)
			}
			if _, err := fmt.Fprint(t.out, value); err != nil {
				return err
			}
		}
		if len(row) > 0 {
			if _, err := fmt.Fprint(t.out, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *TablePrinter) calculateColumnWidths(delimiterSize int) []int {
	numColumns := len(t.rows[0])
	maxColumnWidths := make([]int, numColumns)
	columnWidths := make([]int, numColumns)
	for _, row := range t.rows {
		for column, field := range row {
			width := text.DisplayWidth(field.text)
			if width > maxColumnWidths[column] {
				maxColumnWidths[column] = width
			}
			if field.truncateFunc == nil && width > columnWidths[column] {
				columnWidths[column] = width
			}
		}
	}
	availableWidth := func() int {
		usedWidth := 0
		for _, width := range columnWidths {
			usedWidth += width
		}
		return t.maxWidth - delimiterSize*(numColumns-1) - usedWidth
	}
	fixedColumns := func() int {
		count := 0
		for _, width := range columnWidths {
			if width > 0 {
				count++
			}
		}
		return count
	}
	if width := availableWidth(); width > 0 {
		if flexibleColumns := numColumns - fixedColumns(); flexibleColumns > 0 {
			perColumn := width / flexibleColumns
			for column, maximum := range maxColumnWidths {
				if maximum < perColumn {
					columnWidths[column] = maximum
				}
			}
		}
	}
	if flexibleColumns := numColumns - fixedColumns(); flexibleColumns > 0 {
		perColumn := availableWidth() / flexibleColumns
		for column, width := range columnWidths {
			if width == 0 {
				if maximum := maxColumnWidths[column]; maximum < perColumn {
					columnWidths[column] = maximum
				} else if perColumn > 0 {
					columnWidths[column] = perColumn
				}
			}
		}
	}
	if width := availableWidth(); width > 0 {
		for column, maximum := range maxColumnWidths {
			remaining := maximum - columnWidths[column]
			addition := width
			if remaining < addition {
				addition = remaining
			}
			columnWidths[column] += addition
			width -= addition
			if width <= 0 {
				break
			}
		}
	}
	return columnWidths
}
