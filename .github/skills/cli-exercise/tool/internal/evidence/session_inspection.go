package evidence

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"slices"

	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

func inspectionRangeSamples(first, last int) []int {
	return slices.Compact([]int{first, first + (last-first+1)/2, last})
}

func renderedInspectionFrames(entries []renderedOccurrence, count, hold int) []int {
	frames := []int{}
	for first := 0; first < len(entries); {
		last := first
		for last+1 < len(entries) && entries[last+1].Image == entries[first].Image &&
			entries[last+1].SourceState == entries[first].SourceState &&
			entries[last+1].NoteTimeSeconds == entries[first].NoteTimeSeconds {
			last++
		}
		frames = append(frames, inspectionRangeSamples(entries[first].FirstFrame, entries[last].LastFrame)...)
		first = last + 1
	}
	if hold > 0 {
		frames = append(frames, inspectionRangeSamples(count-hold, count-1)...)
	}
	slices.Sort(frames)
	return slices.Compact(frames)
}

func sessionInspectionFrames(chapters []sessionChapter, mode string) ([]int, error) {
	if !slices.Contains([]string{"sampled", "all"}, mode) {
		return nil, fmt.Errorf("--inspection must be sampled or all")
	}
	frames, offset := []int{}, 0
	for _, chapter := range chapters {
		clip := chapter.Evidence.Rendering
		if clip.Frames <= 0 || clip.Frames > 1_000_000-offset {
			return nil, fmt.Errorf("session inspection requires a bounded, nonempty timeline")
		}
		if mode == "all" {
			selected := clip.inspectionFrames
			if chapter.Kind == "overview" {
				selected = inspectionRangeSamples(0, clip.Frames-1)
			}
			if len(selected) == 0 || selected[0] != 0 || selected[len(selected)-1] != clip.Frames-1 {
				return nil, fmt.Errorf("all session inspection requires complete chapter frame selections")
			}
			for index, frame := range selected {
				if frame < 0 || frame >= clip.Frames || index > 0 && frame <= selected[index-1] {
					return nil, fmt.Errorf("chapter inspection frame selections are invalid")
				}
				frames = append(frames, offset+frame)
			}
		}
		offset += clip.Frames
	}
	if offset == 0 {
		return nil, fmt.Errorf("session inspection requires a bounded, nonempty timeline")
	}
	frames = append(frames, inspectionRangeSamples(0, offset-1)...)
	slices.Sort(frames)
	return slices.Compact(frames), nil
}

func readSessionInspectionFrames(ctx context.Context, reader io.Reader, width, height, count int, selected []int, save func(int, *image.RGBA) error) error {
	if width <= 0 || height <= 0 || width > 64_000_000/height || count <= 0 || count > 1_000_000 {
		return fmt.Errorf("session inspection requires bounded geometry and frame count")
	}
	for index, frame := range selected {
		if frame < 0 || frame >= count || index > 0 && frame <= selected[index-1] {
			return fmt.Errorf("session inspection frame selection is invalid")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reader = sessionMediaReader{context: ctx, reader: reader}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	next := 0
	for frame := range count {
		if next < len(selected) && selected[next] == frame {
			if _, err := io.ReadFull(reader, canvas.Pix); err != nil {
				return fmt.Errorf("decode session frame %d: %w", frame, err)
			}
			if err := save(frame, canvas); err != nil {
				return err
			}
			next++
		} else if _, err := io.CopyN(io.Discard, reader, int64(len(canvas.Pix))); err != nil {
			return fmt.Errorf("skip session frame %d: %w", frame, err)
		}
	}
	var extra [1]byte
	if n, err := reader.Read(extra[:]); n != 0 || err != io.EOF {
		return errors.Join(fmt.Errorf("decoded session frame count differs from the video"), err)
	}
	return ctx.Err()
}

func decodeSessionInspection(ctx context.Context, filename string, tools recording.Tools, environment []string,
	width, height, count int, selected []int, store *imageStore) (map[int]string, error) {
	bounded, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := runCommand(bounded, []string{
			tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", filename, "-map", "0:v:0", "-an",
			"-fps_mode", "passthrough", "-pix_fmt", "rgba", "-f", "rawvideo", "-",
		}, environment, filepath.Dir(filename), writer)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	paths := map[int]string{}
	err := readSessionInspectionFrames(bounded, reader, width, height, count, selected, func(frame int, canvas *image.RGBA) error {
		path, err := store.save(canvas)
		if err == nil {
			paths[frame] = path
		}
		return err
	})
	if err != nil {
		cancel()
	}
	return paths, errors.Join(err, reader.Close(), <-done)
}
