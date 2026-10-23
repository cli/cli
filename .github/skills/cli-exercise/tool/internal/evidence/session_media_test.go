package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
	"github.com/stretchr/testify/require"
)

func sessionMediaTestChapters() []sessionChapter {
	return []sessionChapter{
		{
			CaseID: "first", CaseTitle: "Mona's Cafe", RunID: "before", Phase: "setup", Title: "Before",
			Evidence: report{CaseStatus: "failed", Rendering: rendering{
				Status: "complete", Media: map[string]string{"mp4": "first clip's.mp4"},
				Width: 64, Height: 48, Frames: 1, FPS: 30, DurationSeconds: 1.0 / 30,
				CaptureDurationSeconds: 0.025, Timing: "realtime", CapturedStates: 1, VideoStates: 1,
			}},
		},
		{
			CaseID: "second", CaseTitle: "Another case", RunID: "action", Phase: "action", Title: "Inspect",
			Evidence: report{CaseStatus: "passed", Rendering: rendering{
				Status: "complete", Media: map[string]string{"mp4": "second clip.mp4"},
				Width: 96, Height: 64, Frames: 2, FPS: 30, DurationSeconds: 2.0 / 30,
				CaptureDurationSeconds: 0.06, Timing: "realtime", CapturedStates: 2, VideoStates: 2,
			}},
		},
	}
}

func sessionMediaCases(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			change func([]sessionChapter) []sessionChapter
			err    string
		}{
			{name: "valid failed case is still media"},
			{name: "no clips", change: func(_ []sessionChapter) []sessionChapter { return nil }, err: "at least one"},
			{name: "missing MP4", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Media = nil; return c }, err: "complete MP4"},
			{name: "failed rendering", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Status = "failed"; return c }, err: "complete MP4"},
			{name: "render error", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Error = "failed"; return c }, err: "complete MP4"},
			{name: "chapter error", change: func(c []sessionChapter) []sessionChapter { c[0].Error = "missing clip"; return c }, err: "complete MP4"},
			{name: "zero width", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Width = 0; return c }, err: "dimensions"},
			{name: "negative height", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Height = -2; return c }, err: "dimensions"},
			{name: "odd width", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Width = 65; return c }, err: "dimensions"},
			{name: "odd height", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Height = 49; return c }, err: "dimensions"},
			{name: "oversized clip", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Width = 64_000_000; return c }, err: "dimensions"},
			{name: "oversized combined geometry", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.Width, c[0].Evidence.Rendering.Height = 64_000, 1000
				c[1].Evidence.Rendering.Width, c[1].Evidence.Rendering.Height = 1000, 64_000
				return c
			}, err: "padded session geometry"},
			{name: "zero fps", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.FPS = 0; return c }, err: "frame rate"},
			{name: "fps beyond renderer limit", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.FPS = 61; return c }, err: "frame rate"},
			{name: "incompatible fps", change: func(c []sessionChapter) []sessionChapter { c[1].Evidence.Rendering.FPS = 24; return c }, err: "frame rate"},
			{name: "zero frames", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Frames = 0; return c }, err: "frame counts"},
			{name: "negative frames", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Frames = -1; return c }, err: "frame counts"},
			{name: "too many frames", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.Frames = 1_000_001; return c }, err: "one-million-frame"},
			{name: "too many aggregate frames", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.Frames = 1_000_000
				c[0].Evidence.Rendering.DurationSeconds = 1_000_000.0 / 30
				return c
			}, err: "one-million-frame"},
			{name: "zero duration", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.DurationSeconds = 0; return c }, err: "duration differs"},
			{name: "NaN duration", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.DurationSeconds = math.NaN()
				return c
			}, err: "duration differs"},
			{name: "infinite duration", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.DurationSeconds = math.Inf(1)
				return c
			}, err: "duration differs"},
			{name: "inconsistent duration", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.DurationSeconds = 2; return c }, err: "duration differs"},
			{name: "zero capture", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.CaptureDurationSeconds = 0
				return c
			}, err: "capture duration"},
			{name: "negative capture", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.CaptureDurationSeconds = -1
				return c
			}, err: "capture duration"},
			{name: "NaN capture", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.CaptureDurationSeconds = math.NaN()
				return c
			}, err: "capture duration"},
			{name: "capture sum overflow", change: func(c []sessionChapter) []sessionChapter {
				c[0].Evidence.Rendering.CaptureDurationSeconds, c[1].Evidence.Rendering.CaptureDurationSeconds = math.MaxFloat64, math.MaxFloat64
				return c
			}, err: "capture duration"},
			{name: "missing case title", change: func(c []sessionChapter) []sessionChapter { c[0].CaseTitle = ""; return c }, err: "titles"},
			{name: "missing phase", change: func(c []sessionChapter) []sessionChapter { c[0].Phase = ""; return c }, err: "titles"},
			{name: "missing run title", change: func(c []sessionChapter) []sessionChapter { c[0].Title = ""; return c }, err: "titles"},
			{name: "NUL in title", change: func(c []sessionChapter) []sessionChapter { c[0].Title = "before\x00after"; return c }, err: "titles"},
			{name: "carriage return in title", change: func(c []sessionChapter) []sessionChapter { c[0].Title = "before\rafter"; return c }, err: "titles"},
			{name: "oversized title", change: func(c []sessionChapter) []sessionChapter { c[0].Title = strings.Repeat("x", 65_535); return c }, err: "titles"},
			{name: "negative state count", change: func(c []sessionChapter) []sessionChapter { c[0].Evidence.Rendering.CapturedStates = -1; return c }, err: "state counts"},
			{name: "invalid hold", change: func(c []sessionChapter) []sessionChapter {
				value := math.NaN()
				c[0].Evidence.Rendering.EndHoldSeconds = &value
				return c
			}, err: "end hold"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				chapters := sessionMediaTestChapters()
				if tc.change != nil {
					chapters = tc.change(chapters)
				}
				planned, timeline, err := planSessionMedia(chapters)
				if tc.err != "" {
					require.ErrorContains(t, err, tc.err)
					require.Empty(t, planned)
					require.Nil(t, timeline)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 96, planned.Width)
				require.Equal(t, 64, planned.Height)
				require.Equal(t, 3, planned.Frames)
				require.Equal(t, 30, planned.FPS)
				require.Equal(t, 0.1, planned.DurationSeconds)
				require.InDelta(t, 0.085, planned.CaptureDurationSeconds, 0.000001)
				require.Equal(t, 3, planned.CapturedStates)
				require.Equal(t, 3, planned.VideoStates)
				require.Equal(t, "failed", timeline[0].Evidence.CaseStatus)
				require.Equal(t, "passed", timeline[1].Evidence.CaseStatus)
				require.Equal(t, 1.0/30, timeline[0].EndSeconds)
				require.Equal(t, timeline[0].EndSeconds, timeline[1].StartSeconds)
				require.Equal(t, 0.1, timeline[1].EndSeconds)
				require.Zero(t, chapters[0].EndSeconds, "the caller's chapters are not mutated")
			})
		}
	})
	t.Run("formats and selected capabilities", func(t *testing.T) {
		root := t.TempDir()
		tool := filepath.Join(root, "selected-tool")
		require.NoError(t, os.WriteFile(tool, []byte("not executed"), 0o600))
		for _, tc := range []struct {
			name    string
			formats []string
			change  func(*recording.Receipt)
			err     string
		}{
			{name: "no formats", err: "output formats"},
			{name: "unsupported format", formats: []string{"webm"}, err: "requested session format"},
			{name: "not ready", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Status = "blocked" }, err: "ready receipt"},
			{name: "MP4 unavailable", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Checks.Formats["mp4"] = false }, err: "MP4 encoding"},
			{name: "GIF-only still needs MP4", formats: []string{"gif"}, change: func(r *recording.Receipt) { r.Checks.Formats["mp4"] = false }, err: "including for GIF-only"},
			{name: "GIF unavailable", formats: []string{"gif", "mp4"}, change: func(r *recording.Receipt) { r.Checks.Formats["gif"] = false }, err: `format "gif"`},
			{name: "missing ffmpeg", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFmpeg = nil }, err: "absolute ffmpeg"},
			{name: "relative ffmpeg", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFmpeg.Path = "ffmpeg" }, err: "absolute ffmpeg"},
			{name: "missing ffprobe", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFprobe = nil }, err: "absolute ffprobe"},
			{name: "relative ffprobe", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFprobe.Path = "ffprobe" }, err: "absolute ffprobe"},
			{name: "changed ffmpeg", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFmpeg.SHA256 = "changed" }, err: "no longer matches"},
			{name: "changed ffprobe", formats: []string{"mp4"}, change: func(r *recording.Receipt) { r.Tools.FFprobe.SHA256 = "changed" }, err: "no longer matches"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				receipt := recording.Receipt{Status: "ready",
					Checks: recording.CapabilityChecks{Formats: map[string]bool{"mp4": true, "gif": true}},
					Tools:  recording.Tools{FFmpeg: &recording.Executable{Path: tool}, FFprobe: &recording.Executable{Path: tool}},
				}
				if tc.change != nil {
					tc.change(&receipt)
				}
				destination := filepath.Join(t.TempDir(), "output")
				rendered, timeline, err := assembleSession(t.Context(), destination, receipt, sessionMediaTestChapters(), tc.formats, false, "sampled")
				require.ErrorContains(t, err, tc.err)
				require.Empty(t, rendered)
				require.Nil(t, timeline)
				require.NoDirExists(t, destination)
			})
		}
	})
	t.Run("safe copies and no overwrite", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "first clip's.mp4")
		content := []byte("test-owned bytes, not a genuine clip")
		require.NoError(t, os.WriteFile(source, content, 0o600))
		destination := filepath.Join(root, sessionMediaClipName(0))
		require.NoError(t, copySessionMedia(t.Context(), source, destination))
		copied, err := os.ReadFile(destination)
		require.NoError(t, err)
		require.Equal(t, content, copied)
		require.Error(t, copySessionMedia(t.Context(), source, destination))
		original, err := os.ReadFile(source)
		require.NoError(t, err)
		require.Equal(t, content, original)
		require.Error(t, copySessionMedia(t.Context(), filepath.Join(root, "missing"), filepath.Join(root, "missing-copy")))
		require.ErrorContains(t, copySessionMedia(t.Context(), root, filepath.Join(root, "directory-copy")), "regular files")
		empty := filepath.Join(root, "empty.mp4")
		require.NoError(t, os.WriteFile(empty, nil, 0o600))
		require.ErrorContains(t, copySessionMedia(t.Context(), empty, filepath.Join(root, "empty-copy")), "nonempty")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, copySessionMedia(ctx, source, filepath.Join(root, "cancelled-copy")), context.Canceled)
	})
	t.Run("metadata escaping and integer boundaries", func(t *testing.T) {
		chapters := sessionMediaTestChapters()
		chapters[0].CaseTitle = "Mona's = #1; \\notes\nnext"
		chapters[0].Evidence.Rendering.Media["mp4"] = "clip's\nfile '/untrusted.mp4'"
		chapters[0].StartSeconds, chapters[0].EndSeconds = 123, 456
		require.Equal(t, ";FFMETADATA1\n"+
			"[CHAPTER]\nTIMEBASE=1/30\nSTART=0\nEND=1\ntitle=Mona's \\= \\#1\\; \\\\notes\\\nnext - setup: Before\n"+
			"[CHAPTER]\nTIMEBASE=1/30\nSTART=1\nEND=3\ntitle=Another case - action: Inspect\n",
			sessionMediaMetadata(chapters, 30))
		require.Equal(t, "ffconcat version 1.0\n"+
			"file 'clip-000000.mp4'\nduration 0.033333\n"+
			"file 'clip-000001.mp4'\nduration 0.066667\n", sessionMediaConcatList(chapters, 30))
		for _, fps := range []int{1, 24, 30, 60} {
			chapters := make([]sessionChapter, 3000)
			for index := range chapters {
				chapters[index] = sessionMediaTestChapters()[0]
				chapters[index].Evidence.Rendering.FPS = fps
				chapters[index].Evidence.Rendering.DurationSeconds = 1.0 / float64(fps)
			}
			planned, timeline, err := planSessionMedia(chapters)
			require.NoError(t, err)
			require.Equal(t, 3000.0/float64(fps), timeline[len(timeline)-1].EndSeconds)
			require.Equal(t, timeline[len(timeline)-1].EndSeconds, planned.DurationSeconds)
			micros := int64(0)
			for line := range strings.SplitSeq(sessionMediaConcatList(chapters, fps), "\n") {
				if duration, ok := strings.CutPrefix(line, "duration "); ok {
					seconds, err := strconv.ParseFloat(duration, 64)
					require.NoError(t, err)
					micros += int64(math.Round(seconds * 1_000_000))
				}
			}
			require.Equal(t, int64(3000*1_000_000/fps), micros)
		}
	})
	t.Run("frame rates", func(t *testing.T) {
		for _, tc := range []struct {
			rate string
			want bool
		}{
			{"30/1", true}, {"60/2", true}, {"30", true}, {"30000/1001", false},
			{"24/1", false}, {"0/0", false}, {"-30/1", false}, {"N/A", false}, {"", false},
		} {
			require.Equal(t, tc.want, sessionMediaRateMatches(tc.rate, 30), tc.rate)
		}
		require.False(t, sessionMediaRateMatches("0/1", 0))
	})
	t.Run("native chapter validation", func(t *testing.T) {
		chapters := sessionMediaTestChapters()
		for _, tc := range []struct {
			name   string
			change func([]sessionMediaChapterProbe) []sessionMediaChapterProbe
			err    string
		}{
			{name: "millisecond quantization"},
			{name: "exact frame time base", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe {
				c[0].TimeBase, c[0].End = "1/30", 1
				c[1].TimeBase, c[1].Start, c[1].End = "1/30", 1, 3
				return c
			}},
			{name: "missing chapter", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { return c[:1] }, err: "chapter count"},
			{name: "wrong title", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].Tags["title"] = "wrong"; return c }, err: "title"},
			{name: "invalid time base", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].TimeBase = "0/0"; return c }, err: "time base"},
			{name: "coarse time base", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].TimeBase = "1/1"; return c }, err: "time base"},
			{name: "negative start", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].Start = -1; return c }, err: "interval"},
			{name: "empty interval", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].End = 0; return c }, err: "interval"},
			{name: "shifted boundary", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[0].End = 30; return c }, err: "boundary"},
			{name: "incorrect final end", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[1].End = 99; return c }, err: "boundary"},
			{name: "chapter gap", change: func(c []sessionMediaChapterProbe) []sessionMediaChapterProbe { c[1].Start = 34; return c }, err: "not contiguous"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				observed := []sessionMediaChapterProbe{
					{TimeBase: "1/1000", Start: 0, End: 33, Tags: map[string]string{"title": sessionMediaTitle(chapters[0])}},
					{TimeBase: "1/1000", Start: 33, End: 100, Tags: map[string]string{"title": sessionMediaTitle(chapters[1])}},
				}
				if tc.change != nil {
					observed = tc.change(observed)
				}
				data, err := json.Marshal(map[string]any{"chapters": observed})
				require.NoError(t, err)
				actual, err := checkSessionMediaChapters(data, chapters, 30)
				if tc.err != "" {
					require.ErrorContains(t, err, tc.err)
					require.Nil(t, actual)
				} else {
					require.NoError(t, err)
					require.Equal(t, observed, actual)
				}
			})
		}
		_, err := checkSessionMediaChapters([]byte("invalid JSON"), chapters, 30)
		require.ErrorContains(t, err, "invalid session chapters")
	})
	t.Run("explicit native media tools", sessionMediaNativeCase)
}

func sessionMediaNativeCase(t *testing.T) {
	preflight := os.Getenv("CLI_EXERCISE_SESSION_MEDIA_RECEIPT")
	if preflight == "" {
		t.Skip("Set CLI_EXERCISE_SESSION_MEDIA_RECEIPT to an existing ready receipt for test-owned native MP4 fixtures.")
	}
	var receipt recording.Receipt
	_, err := cliutil.ReadJSON(preflight, 1<<20, &receipt)
	require.NoError(t, err)
	require.Equal(t, "ready", receipt.Status)
	require.True(t, receipt.Checks.Formats["mp4"])
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		tool := receipt.Tools.Executable(name)
		require.NotNil(t, tool)
		require.True(t, filepath.IsAbs(tool.Path))
		if tool.SHA256 != "" {
			hash, err := cliutil.SHA256File(tool.Path)
			require.NoError(t, err)
			require.Equal(t, tool.SHA256, hash)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	root := t.TempDir()
	environment, err := rendererEnvironment(root, receipt.Tools)
	require.NoError(t, err)
	chapters := sessionMediaTestChapters()
	chapters[0].CaseTitle = "Mona's = #1; \\notes\nnext"
	chapters[1].Title = strings.Repeat("Long chapter title ", 20)
	colors := make([][3]byte, 0)
	originalHashes := make([]string, len(chapters))
	for index := range chapters {
		clip := &chapters[index].Evidence.Rendering
		clip.Frames = []int{4, 7}[index]
		clip.DurationSeconds = float64(clip.Frames) / float64(clip.FPS)
		pixels := make([]byte, clip.Width*clip.Height*3*clip.Frames)
		for frame := range clip.Frames {
			color := [3]byte{200, byte(35 + frame*14), 40}
			if index == 1 {
				color = [3]byte{30, byte(65 + frame*11), 190}
			}
			colors = append(colors, color)
			for pixel := range clip.Width * clip.Height {
				value := color
				if pixel%clip.Width >= clip.Width-8 {
					value = [3]byte{10, 210, 90}
				}
				offset := (frame*clip.Width*clip.Height + pixel) * 3
				copy(pixels[offset:offset+3], value[:])
			}
		}
		raw := filepath.Join(root, "frames-"+strconv.Itoa(index)+".rgb")
		require.NoError(t, os.WriteFile(raw, pixels, 0o600))
		filename := filepath.Join(root, "fixture clip's "+strconv.Itoa(index)+".mp4")
		args, err := encodingArguments(receipt.Tools.FFmpeg.Path, filename, clip.Width, clip.Height, clip.FPS, "mp4")
		require.NoError(t, err)
		args[slices.Index(args, "-i")+1] = raw
		require.NoError(t, runCommand(ctx, args, environment, root, io.Discard))
		details, err := inspectSessionMediaClip(ctx, filename, receipt.Tools, environment, clip.Width, clip.Height, clip.Frames, clip.FPS)
		require.NoError(t, err)
		clip.Media = map[string]string{"mp4": filename}
		clip.MediaDetails = map[string]mediaInfo{"mp4": details}
		originalHashes[index] = details.SHA256
	}
	for _, tc := range []struct {
		formats  []string
		progress bool
	}{
		{formats: []string{"mp4"}},
		{formats: []string{"gif"}},
		{formats: []string{"mp4", "gif"}},
		{formats: []string{"mp4", "gif"}, progress: true},
	} {
		formats := tc.formats
		name := strings.Join(formats, " and ")
		if tc.progress {
			name += " with continuous progress"
		}
		t.Run(name, func(t *testing.T) {
			if slices.Contains(formats, "gif") && !receipt.Checks.Formats["gif"] {
				t.Skip("The selected receipt does not confirm GIF encoding.")
			}
			directory := filepath.Join(t.TempDir(), "session output's")
			rendered, timeline, err := assembleSession(ctx, directory, receipt, chapters, formats, tc.progress, "sampled")
			require.NoError(t, err)
			require.Equal(t, "complete", rendered.Status)
			require.Len(t, rendered.Media, len(formats))
			require.Len(t, rendered.MediaDetails, len(formats))
			require.Equal(t, 11, rendered.Frames)
			require.Equal(t, 30, rendered.FPS)
			require.Equal(t, 96, rendered.Width)
			require.Equal(t, 64, rendered.Height)
			require.InDelta(t, 0.085, rendered.CaptureDurationSeconds, 0.000001)
			require.Equal(t, 4.0/30, timeline[0].EndSeconds)
			require.Equal(t, timeline[0].EndSeconds, timeline[1].StartSeconds)
			require.Equal(t, 11.0/30, timeline[1].EndSeconds)
			require.Equal(t, "failed", timeline[0].Evidence.CaseStatus)
			require.Zero(t, chapters[0].EndSeconds)
			for index, chapter := range chapters {
				hash, err := cliutil.SHA256File(chapter.Evidence.Rendering.Media["mp4"])
				require.NoError(t, err)
				require.Equal(t, originalHashes[index], hash, "original clips are never overwritten")
			}
			unchanged, err := cliutil.SHA256File(filepath.Join(directory, "assembly", sessionMediaClipName(1)))
			require.NoError(t, err)
			require.Equal(t, originalHashes[1], unchanged, "common geometry requires no re-encoding")
			for _, kind := range formats {
				details := rendered.MediaDetails[kind]
				require.Equal(t, 11, details.Frames)
				require.Equal(t, 96, details.Width)
				require.Equal(t, 64, details.Height)
				require.Positive(t, details.DurationSeconds)
				require.InDelta(t, 11.0/30, details.DurationSeconds, 0.02)
				hash, err := cliutil.SHA256File(rendered.Media[kind])
				require.NoError(t, err)
				require.Equal(t, details.SHA256, hash)
				var decoded bytes.Buffer
				require.NoError(t, runCommand(ctx, []string{
					receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", rendered.Media[kind],
					"-map", "0:v:0", "-an", "-fps_mode", "passthrough", "-pix_fmt", "rgb24", "-f", "rawvideo", "-",
				}, environment, root, &decoded))
				require.Len(t, decoded.Bytes(), 96*64*3*11)
				checkPixel := func(frame, x, y int, color [3]byte, tolerance float64) {
					t.Helper()
					offset := ((frame*64+y)*96 + x) * 3
					for channel, value := range color {
						require.InDelta(t, int(value), int(decoded.Bytes()[offset+channel]), tolerance,
							"%s frame %d pixel (%d,%d), channel %d", kind, frame, x, y, channel)
					}
				}
				for frame, color := range colors {
					checkPixel(frame, 8, 8, color, 8)
					if frame < 4 {
						checkPixel(frame, 60, 8, [3]byte{10, 210, 90}, 8)
						checkPixel(frame, 80, 56, [3]byte{13, 17, 23}, 5)
					}
				}
				if tc.progress {
					checkPixel(0, 48, 62, [3]byte{48, 54, 61}, 8)
					checkPixel(5, 20, 62, [3]byte{121, 192, 255}, 8)
					checkPixel(5, 80, 62, [3]byte{48, 54, 61}, 8)
					checkPixel(10, 90, 62, [3]byte{121, 192, 255}, 8)
					previous := -1
					for frame := range 11 {
						filled := 0
						for x := range 96 {
							offset := ((frame*64+62)*96 + x) * 3
							pixel := decoded.Bytes()[offset : offset+3]
							if pixel[1] > 130 && pixel[2] > 180 {
								filled++
							}
						}
						require.Greater(t, filled, previous, "progress must advance every frame, including across clip boundaries")
						previous = filled
					}
				}
			}
			if !slices.Contains(formats, "mp4") {
				require.NoFileExists(t, filepath.Join(directory, "session.mp4"))
				require.Equal(t, rendered.MediaDetails["gif"].DurationSeconds, rendered.DurationSeconds)
			} else {
				require.Equal(t, rendered.MediaDetails["mp4"].DurationSeconds, rendered.DurationSeconds)
			}
			require.NotNil(t, rendered.Inspection)
			require.Equal(t, "pending", rendered.Inspection.VisualReview)
			hash, err := cliutil.SHA256File(rendered.Inspection.Manifest)
			require.NoError(t, err)
			require.Equal(t, hash, rendered.Inspection.ManifestSHA256)
			var manifest struct {
				Frames           map[string]string          `json:"frames"`
				EmbeddedChapters []sessionMediaChapterProbe `json:"embeddedChapters"`
				VisualReview     string                     `json:"visualReview"`
			}
			_, err = cliutil.ReadJSON(rendered.Inspection.Manifest, 1<<20, &manifest)
			require.NoError(t, err)
			require.Equal(t, "pending", manifest.VisualReview)
			require.Len(t, manifest.EmbeddedChapters, 2)
			for index, chapter := range manifest.EmbeddedChapters {
				require.Equal(t, sessionMediaTitle(chapters[index]), chapter.Tags["title"])
			}
			require.Len(t, manifest.Frames, 3*len(formats))
			for _, kind := range formats {
				require.Len(t, rendered.Inspection.EncodedSamples[kind], 3)
				for _, relative := range rendered.Inspection.EncodedSamples[kind] {
					filename := filepath.Join(rendered.Inspection.Directory, filepath.FromSlash(relative))
					hash, err := cliutil.SHA256File(filename)
					require.NoError(t, err)
					require.Equal(t, manifest.Frames[relative], hash)
					data, err := os.ReadFile(filename)
					require.NoError(t, err)
					config, err := png.DecodeConfig(bytes.NewReader(data))
					require.NoError(t, err)
					require.Equal(t, 96, config.Width)
					require.Equal(t, 64, config.Height)
				}
			}
		})
	}
	t.Run("decoded metadata mismatch is a failure", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			change func([]sessionChapter)
			err    string
		}{
			{name: "frame count", change: func(c []sessionChapter) {
				c[0].Evidence.Rendering.Frames++
				c[0].Evidence.Rendering.DurationSeconds = float64(c[0].Evidence.Rendering.Frames) / 30
			}, err: "frame count"},
			{name: "actual frame rate", change: func(c []sessionChapter) {
				for index := range c {
					c[index].Evidence.Rendering.FPS = 25
					c[index].Evidence.Rendering.DurationSeconds = float64(c[index].Evidence.Rendering.Frames) / 25
				}
			}, err: "frame rate"},
			{name: "actual geometry", change: func(c []sessionChapter) {
				c[0].Evidence.Rendering.Width += 2
			}, err: "dimensions"},
			{name: "invalid MP4", change: func(c []sessionChapter) {
				c[0].Evidence.Rendering.Media["mp4"] = filepath.Join(root, "frames-0.rgb")
			}, err: "inspect chapter"},
			{name: "changed bytes", change: func(c []sessionChapter) {
				details := c[0].Evidence.Rendering.MediaDetails["mp4"]
				details.SHA256 = "changed"
				c[0].Evidence.Rendering.MediaDetails["mp4"] = details
			}, err: "recorded media details"},
			{name: "missing clip", change: func(c []sessionChapter) {
				c[0].Evidence.Rendering.Media["mp4"] = filepath.Join(root, "missing.mp4")
			}, err: "copy chapter"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				input := slices.Clone(chapters)
				for index := range input {
					input[index].Evidence.Rendering.Media = maps.Clone(input[index].Evidence.Rendering.Media)
					input[index].Evidence.Rendering.MediaDetails = maps.Clone(input[index].Evidence.Rendering.MediaDetails)
				}
				tc.change(input)
				directory := filepath.Join(t.TempDir(), "output")
				rendered, timeline, err := assembleSession(ctx, directory, receipt, input, []string{"mp4"}, false, "sampled")
				require.ErrorContains(t, err, tc.err)
				require.Empty(t, rendered)
				require.Nil(t, timeline)
				require.NoFileExists(t, filepath.Join(directory, "session.mp4"))
			})
		}
	})
	t.Run("existing output is not overwritten", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "output")
		require.NoError(t, os.Mkdir(directory, 0o700))
		filename := filepath.Join(directory, "session.mp4")
		require.NoError(t, os.WriteFile(filename, []byte("preserve this existing file"), 0o600))
		rendered, timeline, err := assembleSession(ctx, directory, receipt, chapters, []string{"mp4"}, false, "sampled")
		require.ErrorContains(t, err, "already exists")
		require.Empty(t, rendered)
		require.Nil(t, timeline)
		content, err := os.ReadFile(filename)
		require.NoError(t, err)
		require.Equal(t, "preserve this existing file", string(content))
	})
	t.Run("cancelled assembly is not a partial success", func(t *testing.T) {
		cancelled, stop := context.WithCancel(ctx)
		stop()
		directory := filepath.Join(t.TempDir(), "output")
		rendered, timeline, err := assembleSession(cancelled, directory, receipt, chapters, []string{"mp4"}, false, "sampled")
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, rendered)
		require.Nil(t, timeline)
		require.NoDirExists(t, directory)
	})
}
