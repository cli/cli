package evidence

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"slices"
	"testing"

	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
)

func sessionInspectionCases(t *testing.T) {
	t.Run("state and annotation changes retain representative frames", func(t *testing.T) {
		note := .2
		entries := []renderedOccurrence{
			{Image: "same.png", FirstFrame: 0, LastFrame: 0, SourceState: 0},
			{Image: "same.png", FirstFrame: 1, LastFrame: 3, SourceState: 0},
			{Image: "same.png", FirstFrame: 4, LastFrame: 5, SourceState: 0, NoteTimeSeconds: &note},
			{Image: "same.png", FirstFrame: 6, LastFrame: 11, SourceState: 1, NoteTimeSeconds: &note},
		}
		require.Equal(t, []int{0, 2, 3, 4, 5, 6, 8, 9, 10, 11},
			renderedInspectionFrames(entries, 12, 4))
		require.Empty(t, renderedInspectionFrames(nil, 0, 0))
	})
	t.Run("overview pages chapter boundaries and selections", func(t *testing.T) {
		chapters := []sessionChapter{
			{Kind: "overview", Evidence: report{Rendering: rendering{Frames: 100}}},
			{Kind: "overview", Evidence: report{Rendering: rendering{Frames: 100}}},
			{Kind: "run", Evidence: report{Rendering: rendering{Frames: 12, inspectionFrames: []int{0, 2, 3, 4, 5, 6, 9, 11}}}},
			{Kind: "run", Evidence: report{Rendering: rendering{Frames: 12, inspectionFrames: []int{0, 5, 11}}}},
		}
		sampled, err := sessionInspectionFrames(chapters, "sampled")
		require.NoError(t, err)
		require.Equal(t, []int{0, 112, 223}, sampled)
		all, err := sessionInspectionFrames(chapters, "all")
		require.NoError(t, err)
		require.Equal(t, []int{0, 50, 99, 100, 112, 150, 199, 200, 202, 203, 204, 205, 206, 209, 211, 212, 217, 223}, all)
		require.True(t, slices.IsSorted(all))
	})
	t.Run("invalid selections fail instead of sampling", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			count    int
			selected []int
			err      string
		}{
			{"missing", 12, nil, "complete chapter"},
			{"missing start", 12, []int{1, 6, 11}, "complete chapter"},
			{"missing end", 12, []int{0, 6, 10}, "complete chapter"},
			{"negative", 12, []int{0, -1, 11}, "invalid"},
			{"outside clip", 12, []int{0, 13, 11}, "invalid"},
			{"duplicate", 12, []int{0, 0, 11}, "invalid"},
			{"unordered", 12, []int{0, 6, 3, 11}, "invalid"},
			{"empty clip", 0, nil, "bounded"},
			{"frame limit", 1_000_001, nil, "bounded"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := sessionInspectionFrames([]sessionChapter{{Evidence: report{Rendering: rendering{
					Frames: tc.count, inspectionFrames: tc.selected,
				}}}}, "all")
				require.ErrorContains(t, err, tc.err)
			})
		}
		_, err := sessionInspectionFrames(nil, "all")
		require.ErrorContains(t, err, "nonempty timeline")
		for _, mode := range []string{"", "none", "ALL"} {
			_, err := sessionInspectionFrames(nil, mode)
			require.ErrorContains(t, err, "sampled or all")
			_, _, err = assembleSession(t.Context(), "unused", recording.Receipt{}, nil, nil, false, mode)
			require.ErrorContains(t, err, "sampled or all")
		}
	})
	t.Run("bounded decoding and repeated images", func(t *testing.T) {
		store, err := newImageStore(t.TempDir())
		require.NoError(t, err)
		reader := &inspectionPixelReader{remaining: 100_000 * 4}
		selected := []int{0, 50_000, 99_999}
		frames, paths := []int{}, []string{}
		require.NoError(t, readSessionInspectionFrames(t.Context(), reader, 1, 1, 100_000, selected, func(frame int, canvas *image.RGBA) error {
			path, err := store.save(canvas)
			frames, paths = append(frames, frame), append(paths, path)
			return err
		}))
		require.Equal(t, selected, frames)
		require.Equal(t, paths[0], paths[2])
		require.Len(t, store.images, 1)
		require.LessOrEqual(t, reader.maxRead, 8192, "unselected pixels are streamed, not retained")
		for _, tc := range []struct {
			name                  string
			width, height, frames int
			selected              []int
			bytes                 int
		}{
			{"frame bound", 1, 1, 1_000_001, nil, 0},
			{"geometry bound", 64_000_002, 2, 1, nil, 0},
			{"zero width", 0, 2, 1, nil, 0},
			{"negative selection", 1, 1, 1, []int{-1}, 0},
			{"outside video", 1, 1, 1, []int{1}, 0},
			{"duplicate selection", 1, 1, 1, []int{0, 0}, 0},
			{"unordered selection", 1, 1, 2, []int{1, 0}, 0},
			{"short frame", 1, 1, 1, []int{0}, 1},
			{"missing unselected frame", 1, 1, 2, []int{0}, 4},
			{"extra frame", 1, 1, 1, []int{0}, 8},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := readSessionInspectionFrames(t.Context(), bytes.NewReader(make([]byte, tc.bytes)),
					tc.width, tc.height, tc.frames, tc.selected, func(int, *image.RGBA) error { return nil })
				require.Error(t, err)
			})
		}
		cancelled, cancel := context.WithCancel(t.Context())
		err = readSessionInspectionFrames(cancelled, bytes.NewReader(make([]byte, 8)), 1, 1, 2, []int{0, 1},
			func(int, *image.RGBA) error { cancel(); return nil })
		require.ErrorIs(t, err, context.Canceled)
		err = readSessionInspectionFrames(t.Context(), bytes.NewReader(make([]byte, 4)), 1, 1, 1, []int{0},
			func(int, *image.RGBA) error { return errors.New("image write failed") })
		require.ErrorContains(t, err, "image write failed")
	})
	t.Run("native final inspection", sessionInspectionNativeCase)
}

type inspectionPixelReader struct {
	remaining int
	maxRead   int
}

func (reader *inspectionPixelReader) Read(data []byte) (int, error) {
	reader.maxRead = max(reader.maxRead, len(data))
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := min(len(data), reader.remaining)
	for index := range count {
		data[index] = 255
	}
	reader.remaining -= count
	return count, nil
}
