package loglist

import "sync"

var lintlist struct {
	sync.Once
	list List
	err  error
}

// InitLintList creates and stores a loglist intended for linting (i.e. with
// purpose Validation). We have to store this in a global because the zlint
// framework doesn't (yet) support configuration, so the e_scts_from_same_operator
// lint cannot load a log list on its own. Instead, we have the CA call this
// initialization function at startup, and have the lint call the getter below
// to get access to the cached list.
func InitLintList(path string) error {
	lintlist.Do(func() {
		l, err := New(path)
		if err != nil {
			lintlist.err = err
			return
		}

		l, err = l.forPurpose(Validation)
		if err != nil {
			lintlist.err = err
			return
		}

		lintlist.list = l
	})

	return lintlist.err
}

// GetLintList returns the log list initialized by InitLintList. This must
// only be called after InitLintList has been called on the same (or parent)
// goroutine.
func GetLintList() List {
	return lintlist.list
}
