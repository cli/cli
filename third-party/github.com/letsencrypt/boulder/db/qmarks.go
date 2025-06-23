package db

import "strings"

// QuestionMarks returns a string consisting of N question marks, joined by
// commas. If n is <= 0, panics.
func QuestionMarks(n int) string {
	if n <= 0 {
		panic("db.QuestionMarks called with n <=0")
	}
	var qmarks strings.Builder
	qmarks.Grow(2 * n)
	for i := range n {
		if i == 0 {
			qmarks.WriteString("?")
		} else {
			qmarks.WriteString(",?")
		}
	}
	return qmarks.String()
}
