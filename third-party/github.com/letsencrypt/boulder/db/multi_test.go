package db

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestNewMulti(t *testing.T) {
	_, err := NewMultiInserter("", []string{"colA"}, "")
	test.AssertError(t, err, "Empty table name should fail")

	_, err = NewMultiInserter("myTable", nil, "")
	test.AssertError(t, err, "Empty fields list should fail")

	mi, err := NewMultiInserter("myTable", []string{"colA"}, "")
	test.AssertNotError(t, err, "Single-column construction should not fail")
	test.AssertEquals(t, len(mi.fields), 1)

	mi, err = NewMultiInserter("myTable", []string{"colA", "colB", "colC"}, "")
	test.AssertNotError(t, err, "Multi-column construction should not fail")
	test.AssertEquals(t, len(mi.fields), 3)

	_, err = NewMultiInserter("", []string{"colA"}, "colB")
	test.AssertError(t, err, "expected error for empty table name")
	_, err = NewMultiInserter("foo\"bar", []string{"colA"}, "colB")
	test.AssertError(t, err, "expected error for invalid table name")

	_, err = NewMultiInserter("myTable", []string{"colA", "foo\"bar"}, "colB")
	test.AssertError(t, err, "expected error for invalid column name")

	_, err = NewMultiInserter("myTable", []string{"colA"}, "foo\"bar")
	test.AssertError(t, err, "expected error for invalid returning column name")
}

func TestMultiAdd(t *testing.T) {
	mi, err := NewMultiInserter("table", []string{"a", "b", "c"}, "")
	test.AssertNotError(t, err, "Failed to create test MultiInserter")

	err = mi.Add([]interface{}{})
	test.AssertError(t, err, "Adding empty row should fail")

	err = mi.Add([]interface{}{"foo"})
	test.AssertError(t, err, "Adding short row should fail")

	err = mi.Add([]interface{}{"foo", "bar", "baz", "bing", "boom"})
	test.AssertError(t, err, "Adding long row should fail")

	err = mi.Add([]interface{}{"one", "two", "three"})
	test.AssertNotError(t, err, "Adding correct-length row shouldn't fail")
	test.AssertEquals(t, len(mi.values), 1)

	err = mi.Add([]interface{}{1, "two", map[string]int{"three": 3}})
	test.AssertNotError(t, err, "Adding heterogeneous row shouldn't fail")
	test.AssertEquals(t, len(mi.values), 2)
	// Note that .Add does *not* enforce that each row is of the same types.
}

func TestMultiQuery(t *testing.T) {
	mi, err := NewMultiInserter("table", []string{"a", "b", "c"}, "")
	test.AssertNotError(t, err, "Failed to create test MultiInserter")
	err = mi.Add([]interface{}{"one", "two", "three"})
	test.AssertNotError(t, err, "Failed to insert test row")
	err = mi.Add([]interface{}{"egy", "kettö", "három"})
	test.AssertNotError(t, err, "Failed to insert test row")

	query, queryArgs := mi.query()
	test.AssertEquals(t, query, "INSERT INTO table (a,b,c) VALUES (?,?,?),(?,?,?)")
	test.AssertDeepEquals(t, queryArgs, []interface{}{"one", "two", "three", "egy", "kettö", "három"})

	mi, err = NewMultiInserter("table", []string{"a", "b", "c"}, "id")
	test.AssertNotError(t, err, "Failed to create test MultiInserter")
	err = mi.Add([]interface{}{"one", "two", "three"})
	test.AssertNotError(t, err, "Failed to insert test row")
	err = mi.Add([]interface{}{"egy", "kettö", "három"})
	test.AssertNotError(t, err, "Failed to insert test row")

	query, queryArgs = mi.query()
	test.AssertEquals(t, query, "INSERT INTO table (a,b,c) VALUES (?,?,?),(?,?,?) RETURNING id")
	test.AssertDeepEquals(t, queryArgs, []interface{}{"one", "two", "three", "egy", "kettö", "három"})
}
