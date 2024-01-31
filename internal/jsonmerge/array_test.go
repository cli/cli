package jsonmerge

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrayMerger_singleEmptyArray(t *testing.T) {
	merger := NewArrayMerger()
	w := &bytes.Buffer{}

	r1 := bytes.NewBufferString(`[]`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `[]`, w.String())

	require.NoError(t, merger.Close())
}

func TestArrayMerger_finalEmptyArray(t *testing.T) {
	merger := NewArrayMerger()
	w := &bytes.Buffer{}

	r1 := bytes.NewBufferString(`["a","b"]`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(8), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `["a","b"`, w.String())

	r2 := bytes.NewBufferString(`[]`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, `["a","b" ]`, w.String())

	require.NoError(t, merger.Close())
}

func TestArrayMerger_multiplePages(t *testing.T) {
	merger := NewArrayMerger()
	w := &bytes.Buffer{}

	r1 := bytes.NewBufferString(`["a","b"]`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(8), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `["a","b"`, w.String())

	r2 := bytes.NewBufferString(`["c","d"]`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(9), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, `["a","b","c","d"]`, w.String())

	require.NoError(t, merger.Close())
}

func TestArrayMerger_emptyObject(t *testing.T) {
	merger := NewArrayMerger()
	w := &bytes.Buffer{}

	r1 := bytes.NewBufferString(`{}`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `{}`, w.String())

	require.NoError(t, merger.Close())
}
