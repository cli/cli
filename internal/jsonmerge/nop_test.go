package jsonmerge

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNopMerger_nothingWritten(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	require.NoError(t, merger.Close())
	assert.Equal(t, ``, w.String())
}

func TestNopMerger_singleEmptyObject(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	r1 := bytes.NewBufferString(`{}`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `{}`, w.String())

	require.NoError(t, merger.Close())
	assert.Equal(t, `{}`, w.String())
}

func TestNopMerger_finalEmptyObject(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	r1 := bytes.NewBufferString(`{"a":1,"b":2}`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(13), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `{"a":1,"b":2}`, w.String())

	r2 := bytes.NewBufferString(`{"c":3}`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(7), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, `{"a":1,"b":2}{"c":3}`, w.String())

	require.NoError(t, merger.Close())
	assert.Equal(t, `{"a":1,"b":2}{"c":3}`, w.String())
}

func TestNopMerger_invalidJSON(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	r1 := bytes.NewBufferString(`invalid`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(7), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `invalid`, w.String())

	require.NoError(t, merger.Close())
	assert.Equal(t, `invalid`, w.String())
}

func TestNopMerger_singleEmptyArray(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	r1 := bytes.NewBufferString(`[]`)
	p1 := merger.NewPage(r1, true)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `[]`, w.String())

	require.NoError(t, merger.Close())
	assert.Equal(t, `[]`, w.String())
}

func TestNopMerger_finalEmptyArray(t *testing.T) {
	w := &bytes.Buffer{}
	merger := NewNopMerger()

	r1 := bytes.NewBufferString(`["a","b"]`)
	p1 := merger.NewPage(r1, false)
	n, err := io.Copy(w, p1)
	require.NoError(t, err)
	assert.Equal(t, int64(9), n)
	assert.NoError(t, p1.Close())
	assert.Equal(t, `["a","b"]`, w.String())

	r2 := bytes.NewBufferString(`[]`)
	p2 := merger.NewPage(r2, true)
	n, err = io.Copy(w, p2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.NoError(t, p2.Close())
	assert.Equal(t, `["a","b"][]`, w.String())

	require.NoError(t, merger.Close())
	assert.Equal(t, `["a","b"][]`, w.String())
}
