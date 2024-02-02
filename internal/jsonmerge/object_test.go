package jsonmerge

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectMerger_nothingWritten(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	require.NoError(t, merger.Close())
	assert.Equal(t, ``, w.String())
}

func TestObjectMerger_singleEmptyObject(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	r1 := bytes.NewBufferString(`{}`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, ``, w.String())

	require.NoError(t, merger.Close())
	assert.JSONEq(t, `{}`, w.String())
}

func TestObjectMerger_finalEmptyObject(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	r1 := bytes.NewBufferString(`{"a":1,"b":2}`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, ``, w.String())

	r2 := bytes.NewBufferString(`{}`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, ``, w.String())

	require.NoError(t, merger.Close())
	assert.JSONEq(t, `{"a":1,"b":2}`, w.String())
}

func TestObjectMerger_multiplePages(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	r1 := bytes.NewBufferString(`{"a":1,"b":2,"arr":["a","b"]}`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, ``, w.String())

	r2 := bytes.NewBufferString(`{"b":3,"c":{"d":4},"arr":["c","d"]}`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, ``, w.String())

	require.NoError(t, merger.Close())
	assert.JSONEq(t, `{"a":1,"b":3,"c":{"d":4},"arr":["a","b","c","d"]}`, w.String())
}

func TestObjectMerger_invalidJSON(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	r1 := bytes.NewBufferString(`invalid`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Error(t, p1.Close())
}

func TestObjectMerger_array(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewObjectMerger(w)

	r1 := bytes.NewBufferString(`[]`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.Error(t, p1.Close())
}
