package label

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type label struct {
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
}

func sortLabels(labels []label, field string, ascending bool) {
	sorter := &labelSorter{
		data:      labels,
		ascending: ascending,
		field:     field,
	}

	sort.Sort(sorter)
}

type labelSorter struct {
	ascending bool
	data      []label
	field     string
}

func (l *labelSorter) Len() int {
	return len(l.data)
}

func (l *labelSorter) Less(i, j int) bool {
	var less bool
	switch l.field {
	case "name":
		less = strings.Compare(l.data[i].Name, l.data[j].Name) < 0
	case "created":
		// Use a secondary sort over the name.
		switch {
		case l.data[i].CreatedAt.Equal(l.data[j].CreatedAt):
			less = strings.Compare(l.data[i].Name, l.data[j].Name) < 0
		default:
			less = l.data[i].CreatedAt.Before(l.data[j].CreatedAt)
		}
	default:
		panic(fmt.Errorf(`cannot sort labels by field %q`, l.field))
	}
	if l.ascending {
		return less
	}
	return !less
}

func (l *labelSorter) Swap(i, j int) {
	l.data[i], l.data[j] = l.data[j], l.data[i]
}
