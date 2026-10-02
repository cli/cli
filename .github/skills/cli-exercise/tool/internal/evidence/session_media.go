package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

func planSessionMedia(chapters []sessionChapter) (rendering, []sessionChapter, error) {
	planned := rendering{}
	if len(chapters) == 0 {
		return planned, nil, fmt.Errorf("session assembly requires at least one complete MP4 clip")
	}
	timeline := slices.Clone(chapters)
	planned.FPS = chapters[0].Evidence.Rendering.FPS
	hold, hasHold := 0.0, true
	for index := range timeline {
		chapter := &timeline[index]
		clip := chapter.Evidence.Rendering
		if chapter.Error != "" || clip.Status != "complete" || clip.Error != "" || clip.Media["mp4"] == "" {
			return rendering{}, nil, fmt.Errorf("chapter %d requires a complete MP4 clip", index+1)
		}
		if clip.Width <= 0 || clip.Height <= 0 || clip.Width%2 != 0 || clip.Height%2 != 0 ||
			clip.Width > 64_000_000/clip.Height {
			return rendering{}, nil, fmt.Errorf("chapter %d has invalid or unbounded MP4 dimensions", index+1)
		}
		if clip.FPS < 1 || clip.FPS > 60 || clip.FPS != planned.FPS {
			return rendering{}, nil, fmt.Errorf("chapter %d has an invalid or incompatible frame rate", index+1)
		}
		if clip.Frames <= 0 || clip.Frames > 1_000_000-planned.Frames {
			return rendering{}, nil, fmt.Errorf("session assembly requires positive clip frame counts within the one-million-frame limit")
		}
		if !finite(clip.DurationSeconds) || clip.DurationSeconds <= 0 ||
			math.Abs(clip.DurationSeconds-float64(clip.Frames)/float64(clip.FPS)) > 0.000001 {
			return rendering{}, nil, fmt.Errorf("chapter %d duration differs from its frame count and frame rate", index+1)
		}
		if !finite(clip.CaptureDurationSeconds) || clip.CaptureDurationSeconds < 0 ||
			clip.CaptureDurationSeconds == 0 && chapter.Kind != "overview" ||
			!finite(planned.CaptureDurationSeconds+clip.CaptureDurationSeconds) {
			return rendering{}, nil, fmt.Errorf("chapter %d has an invalid capture duration", index+1)
		}
		title := sessionMediaTitle(*chapter)
		if chapter.Kind != "overview" && (strings.TrimSpace(chapter.CaseTitle) == "" || strings.TrimSpace(chapter.Phase) == "") ||
			strings.TrimSpace(chapter.Title) == "" || strings.ContainsAny(title, "\x00\r") || len(title) > 65_535 {
			return rendering{}, nil, fmt.Errorf("chapter %d requires representable case, phase, and run titles", index+1)
		}
		maxInt := int(^uint(0) >> 1)
		if clip.CapturedStates < 0 || clip.CapturedStates > maxInt-planned.CapturedStates ||
			clip.VideoStates < 0 || clip.VideoStates > maxInt-planned.VideoStates {
			return rendering{}, nil, fmt.Errorf("chapter %d has invalid state counts", index+1)
		}
		if clip.EndHoldSeconds == nil && chapter.Kind != "overview" {
			hasHold = false
		} else if clip.EndHoldSeconds != nil {
			if !finite(*clip.EndHoldSeconds) || *clip.EndHoldSeconds < 0 || !finite(hold+*clip.EndHoldSeconds) {
				return rendering{}, nil, fmt.Errorf("chapter %d has an invalid end hold", index+1)
			}
			hold += *clip.EndHoldSeconds
		}
		chapter.StartSeconds = float64(planned.Frames) / float64(planned.FPS)
		planned.Frames += clip.Frames
		chapter.EndSeconds = float64(planned.Frames) / float64(planned.FPS)
		planned.Width, planned.Height = max(planned.Width, clip.Width), max(planned.Height, clip.Height)
		planned.CaptureDurationSeconds += clip.CaptureDurationSeconds
		planned.CapturedStates += clip.CapturedStates
		planned.VideoStates += clip.VideoStates
		planned.PresentationHoldSeconds += clip.PresentationHoldSeconds
		if chapter.Kind != "overview" {
			if planned.Timing == "" {
				planned.Timing = clip.Timing
			} else if planned.Timing != clip.Timing {
				planned.Timing = "mixed"
			}
		}
		for _, warning := range clip.Warnings {
			if !slices.Contains(planned.Warnings, warning) {
				planned.Warnings = append(planned.Warnings, warning)
			}
		}
	}
	if planned.Width > 64_000_000/planned.Height {
		return rendering{}, nil, fmt.Errorf("padded session geometry exceeds the bounded raster dimensions")
	}
	planned.DurationSeconds = float64(planned.Frames) / float64(planned.FPS)
	if hasHold {
		planned.EndHoldSeconds = &hold
	}
	return planned, timeline, nil
}

func sessionMediaTitle(chapter sessionChapter) string {
	if chapter.Kind == "overview" {
		return chapter.Title
	}
	return chapter.CaseTitle + " - " + chapter.Phase + ": " + chapter.Title
}

func sessionMediaMetadata(chapters []sessionChapter, fps int) string {
	var output strings.Builder
	output.WriteString(";FFMETADATA1\n")
	escape := strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`, "\n", "\\\n")
	frame := 0
	for _, chapter := range chapters {
		end := frame + chapter.Evidence.Rendering.Frames
		fmt.Fprintf(&output, "[CHAPTER]\nTIMEBASE=1/%d\nSTART=%d\nEND=%d\ntitle=%s\n",
			fps, frame, end, escape.Replace(sessionMediaTitle(chapter)))
		frame = end
	}
	return output.String()
}

func sessionMediaClipName(index int) string {
	return fmt.Sprintf("clip-%06d.mp4", index)
}

func sessionMediaConcatList(chapters []sessionChapter, fps int) string {
	var output strings.Builder
	output.WriteString("ffconcat version 1.0\n")
	frame, previous := int64(0), int64(0)
	for index, chapter := range chapters {
		frame += int64(chapter.Evidence.Rendering.Frames)
		// Rounding cumulative boundaries avoids accumulating one rounding error per clip.
		next := (frame*1_000_000 + int64(fps)/2) / int64(fps)
		duration := next - previous
		fmt.Fprintf(&output, "file '%s'\nduration %d.%06d\n", sessionMediaClipName(index), duration/1_000_000, duration%1_000_000)
		previous = next
	}
	return output.String()
}

type sessionMediaReader struct {
	context context.Context
	reader  io.Reader
}

func (reader sessionMediaReader) Read(buffer []byte) (int, error) {
	if err := reader.context.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func copySessionMedia(ctx context.Context, source, destination string) (err error) {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("session clips must be nonempty regular files, not symbolic links")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	_, err = io.Copy(output, sessionMediaReader{context: ctx, reader: input})
	return err
}

func sessionMediaRateMatches(rate string, fps int) bool {
	value, ok := new(big.Rat).SetString(rate)
	return ok && fps > 0 && value.Cmp(big.NewRat(int64(fps), 1)) == 0
}

func inspectSessionMediaClip(ctx context.Context, filename string, tools recording.Tools, environment []string, width, height, frames, fps int) (mediaInfo, error) {
	info, err := inspectMedia(ctx, filename, "mp4", tools, environment, width, height, frames)
	if err != nil {
		return info, err
	}
	if !sessionMediaRateMatches(info.FrameRate, fps) ||
		math.Abs(info.DurationSeconds-float64(frames)/float64(fps)) > 0.001001 {
		return info, fmt.Errorf("MP4 frame rate or duration differs from the session timeline")
	}
	return info, nil
}

type sessionMediaChapterProbe struct {
	TimeBase string            `json:"time_base"`
	Start    int64             `json:"start"`
	End      int64             `json:"end"`
	Tags     map[string]string `json:"tags"`
}

func checkSessionMediaChapters(data []byte, chapters []sessionChapter, fps int) ([]sessionMediaChapterProbe, error) {
	var observed struct {
		Chapters []sessionMediaChapterProbe `json:"chapters"`
	}
	if err := json.Unmarshal(data, &observed); err != nil {
		return nil, fmt.Errorf("ffprobe returned invalid session chapters: %w", err)
	}
	if len(observed.Chapters) != len(chapters) || len(chapters) == 0 || fps <= 0 {
		return nil, fmt.Errorf("embedded MP4 chapter count differs from the session timeline")
	}
	frame := int64(0)
	previousEnd := new(big.Rat)
	for index, chapter := range observed.Chapters {
		base, ok := new(big.Rat).SetString(chapter.TimeBase)
		if !ok || base.Sign() <= 0 || base.Cmp(big.NewRat(1, int64(fps))) > 0 ||
			chapter.Start < 0 || chapter.End <= chapter.Start || chapter.Tags["title"] != sessionMediaTitle(chapters[index]) {
			return nil, fmt.Errorf("embedded MP4 chapter %d has an invalid time base, interval, or title", index+1)
		}
		start := new(big.Rat).Mul(big.NewRat(chapter.Start, 1), base)
		if start.Cmp(previousEnd) != 0 {
			return nil, fmt.Errorf("embedded MP4 chapter %d is not contiguous with the session timeline", index+1)
		}
		for _, boundary := range []struct {
			tick  int64
			frame int64
		}{
			{chapter.Start, frame},
			{chapter.End, frame + int64(chapters[index].Evidence.Rendering.Frames)},
		} {
			actual := new(big.Rat).Mul(big.NewRat(boundary.tick, 1), base)
			difference := new(big.Rat).Sub(actual, big.NewRat(boundary.frame, int64(fps)))
			// MP4 chapter tracks quantize the explicit frame time base to container ticks.
			if difference.Abs(difference).Cmp(base) >= 0 {
				return nil, fmt.Errorf("embedded MP4 chapter %d boundary differs from the session timeline", index+1)
			}
		}
		previousEnd.Mul(big.NewRat(chapter.End, 1), base)
		frame += int64(chapters[index].Evidence.Rendering.Frames)
	}
	return observed.Chapters, nil
}

func assembleSession(ctx context.Context, directory string, receipt recording.Receipt, chapters []sessionChapter, formats []string, progress bool, inspectionMode string) (rendering, []sessionChapter, error) {
	empty := rendering{}
	if !slices.Contains([]string{"sampled", "all"}, inspectionMode) {
		return empty, nil, fmt.Errorf("--inspection must be sampled or all")
	}
	planned, timeline, err := planSessionMedia(chapters)
	if err != nil {
		return empty, nil, err
	}
	if len(formats) == 0 {
		return empty, nil, fmt.Errorf("session output formats must be gif, mp4, or both")
	}
	if receipt.Status != "ready" || !receipt.Checks.Formats["mp4"] {
		return empty, nil, fmt.Errorf("session assembly requires a ready receipt confirming MP4 encoding, including for GIF-only output")
	}
	for _, kind := range formats {
		if !slices.Contains([]string{"gif", "mp4"}, kind) || !receipt.Checks.Formats[kind] {
			return empty, nil, fmt.Errorf("ready receipt does not confirm requested session format %q", kind)
		}
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		selected := receipt.Tools.Executable(name)
		if selected == nil || !filepath.IsAbs(selected.Path) {
			return empty, nil, fmt.Errorf("receipt must select an absolute %s executable", name)
		}
		if selected.SHA256 != "" {
			hash, err := cliutil.SHA256File(selected.Path)
			if err != nil || hash != selected.SHA256 {
				return empty, nil, fmt.Errorf("selected %s no longer matches the ready receipt", name)
			}
		}
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(max(120, min(1800, planned.DurationSeconds*2+30)))*time.Second)
	defer cancel()
	if err := bounded.Err(); err != nil {
		return empty, nil, err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return empty, nil, err
	}
	if err := cliutil.PrivateDirectory(directory); err != nil {
		return empty, nil, err
	}
	for _, kind := range []string{"mp4", "gif"} {
		if kind == "gif" && !slices.Contains(formats, kind) {
			continue
		}
		filename := filepath.Join(directory, "session."+kind)
		if _, err := os.Lstat(filename); err == nil {
			return empty, nil, fmt.Errorf("session output %q already exists", filepath.Base(filename))
		} else if !os.IsNotExist(err) {
			return empty, nil, err
		}
	}
	staging := filepath.Join(directory, "assembly")
	if err := os.Mkdir(staging, 0o700); err != nil {
		return empty, nil, err
	}
	environment, err := rendererEnvironment(directory, receipt.Tools)
	if err != nil {
		return empty, nil, err
	}
	clips := make([]map[string]any, 0, len(timeline))
	for index, chapter := range timeline {
		clip := chapter.Evidence.Rendering
		source := filepath.Join(staging, fmt.Sprintf("source-%06d.mp4", index))
		if err := copySessionMedia(bounded, clip.Media["mp4"], source); err != nil {
			return empty, nil, fmt.Errorf("copy chapter %d MP4: %w", index+1, err)
		}
		original, err := inspectSessionMediaClip(bounded, source, receipt.Tools, environment, clip.Width, clip.Height, clip.Frames, planned.FPS)
		if err != nil {
			return empty, nil, fmt.Errorf("inspect chapter %d MP4: %w", index+1, err)
		}
		if claimed, exists := clip.MediaDetails["mp4"]; exists &&
			(claimed.SHA256 != original.SHA256 || claimed.Bytes != original.Bytes || claimed.Width != original.Width ||
				claimed.Height != original.Height || claimed.Frames != original.Frames ||
				claimed.DurationSeconds != original.DurationSeconds || !sessionMediaRateMatches(claimed.FrameRate, planned.FPS)) {
			return empty, nil, fmt.Errorf("chapter %d MP4 differs from its recorded media details", index+1)
		}
		normalized := filepath.Join(staging, sessionMediaClipName(index))
		padded := original
		if clip.Width == planned.Width && clip.Height == planned.Height {
			if err := os.Rename(source, normalized); err != nil {
				return empty, nil, err
			}
		} else {
			if err := runCommand(bounded, []string{
				receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-n", "-i", source,
				"-map", "0:v:0", "-an", "-map_metadata", "-1", "-map_chapters", "-1",
				"-vf", fmt.Sprintf("pad=%d:%d:0:0:color=0x0d1117", planned.Width, planned.Height),
				"-fps_mode", "passthrough", "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p",
				"-movflags", "+faststart", normalized,
			}, environment, directory, io.Discard); err != nil {
				return empty, nil, fmt.Errorf("pad chapter %d MP4: %w", index+1, err)
			}
			padded, err = inspectSessionMediaClip(bounded, normalized, receipt.Tools, environment, planned.Width, planned.Height, clip.Frames, planned.FPS)
			if err != nil {
				return empty, nil, fmt.Errorf("inspect padded chapter %d MP4: %w", index+1, err)
			}
		}
		clips = append(clips, map[string]any{
			"caseId": chapter.CaseID, "runId": chapter.RunID, "source": original, "normalized": padded,
		})
	}
	selected, err := sessionInspectionFrames(timeline, inspectionMode)
	if err != nil {
		return empty, nil, err
	}
	list := filepath.Join(staging, "clips.ffconcat")
	metadata := filepath.Join(staging, "chapters.ffmetadata")
	for path, data := range map[string]string{
		list: sessionMediaConcatList(timeline, planned.FPS), metadata: sessionMediaMetadata(timeline, planned.FPS),
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			return empty, nil, err
		}
	}
	mp4 := filepath.Join(directory, "session.mp4")
	arguments := []string{
		receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-n", "-f", "concat", "-safe", "1",
		"-protocol_whitelist", "file", "-i", list, "-f", "ffmetadata", "-i", metadata,
	}
	if progress {
		arguments = append(arguments, "-filter_complex", sessionProgressFilter(planned),
			"-map", "[video]", "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", "-fps_mode", "passthrough")
	} else {
		arguments = append(arguments, "-map", "0:v:0", "-c:v", "copy")
	}
	arguments = append(arguments, "-an", "-map_metadata", "1", "-map_chapters", "1",
		"-movflags", "+faststart+disable_chpl", mp4)
	if err := runCommand(bounded, arguments, environment, directory, io.Discard); err != nil {
		return empty, nil, fmt.Errorf("assemble session MP4: %w", err)
	}
	mp4Info, err := inspectSessionMediaClip(bounded, mp4, receipt.Tools, environment, planned.Width, planned.Height, planned.Frames, planned.FPS)
	if err != nil {
		return empty, nil, fmt.Errorf("inspect assembled session MP4: %w", err)
	}
	var probe bytes.Buffer
	if err := runCommand(bounded, []string{
		receipt.Tools.FFprobe.Path, "-v", "error", "-show_chapters", "-of", "json", mp4,
	}, environment, directory, &probe); err != nil {
		return empty, nil, err
	}
	embedded, err := checkSessionMediaChapters(probe.Bytes(), timeline, planned.FPS)
	if err != nil {
		return empty, nil, err
	}
	planned.Media, planned.MediaDetails = map[string]string{}, map[string]mediaInfo{}
	if slices.Contains(formats, "mp4") {
		planned.Media["mp4"], planned.MediaDetails["mp4"] = mp4, mp4Info
	}
	if slices.Contains(formats, "gif") {
		gif := filepath.Join(directory, "session.gif")
		if err := runCommand(bounded, []string{
			receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-n", "-i", mp4, "-an",
			"-map_metadata", "-1", "-map_chapters", "-1", "-filter_complex", gifFilter,
			"-fps_mode", "passthrough", "-c:v", "gif", gif,
		}, environment, directory, io.Discard); err != nil {
			return empty, nil, fmt.Errorf("encode session GIF: %w", err)
		}
		details, err := inspectMedia(bounded, gif, "gif", receipt.Tools, environment, planned.Width, planned.Height, planned.Frames)
		if err != nil {
			return empty, nil, fmt.Errorf("inspect session GIF: %w", err)
		}
		planned.Media["gif"], planned.MediaDetails["gif"] = gif, details
	}
	inspection := filepath.Join(directory, "inspection")
	if err := os.Mkdir(inspection, 0o700); err != nil {
		return empty, nil, err
	}
	store, err := newImageStore(inspection)
	if err != nil {
		return empty, nil, err
	}
	encoded, images, unique := map[string][]string{}, map[string]string{}, map[string]bool{}
	decoded := map[string]map[int]string{}
	for _, kind := range []string{"mp4", "gif"} {
		filename, exists := planned.Media[kind]
		if !exists {
			continue
		}
		paths := map[int]string{}
		if inspectionMode == "all" {
			paths, err = decodeSessionInspection(bounded, filename, receipt.Tools, environment,
				planned.Width, planned.Height, planned.Frames, selected, store)
		} else {
			var samples []string
			samples, err = decodedSamples(bounded, filename, kind, receipt.Tools, environment,
				planned.Width, planned.Height, planned.Frames, inspection)
			for index, path := range samples {
				paths[selected[index]] = path
			}
		}
		if err != nil {
			return empty, nil, err
		}
		decoded[kind] = paths
		for _, frame := range inspectionRangeSamples(0, planned.Frames-1) {
			encoded[kind] = append(encoded[kind], paths[frame])
		}
		for _, path := range paths {
			if _, exists := images[path]; exists {
				continue
			}
			hash, err := cliutil.SHA256File(filepath.Join(inspection, filepath.FromSlash(path)))
			if err != nil {
				return empty, nil, err
			}
			images[path], unique[hash] = hash, true
		}
	}
	manifest := filepath.Join(inspection, "manifest.json")
	if err := cliutil.WriteJSON(manifest, map[string]any{
		"mode": inspectionMode, "frames": images, "media": planned.MediaDetails, "fps": planned.FPS, "timing": planned.Timing,
		"timelineDurationSeconds": planned.DurationSeconds, "encodedSamples": encoded, "decodedFrames": decoded,
		"assembledMP4": mp4Info, "embeddedChapters": embedded, "clips": clips, "visualReview": "pending",
		"continuousProgress": progress,
	}); err != nil {
		return empty, nil, err
	}
	hash, err := cliutil.SHA256File(manifest)
	if err != nil {
		return empty, nil, err
	}
	imageCount := len(unique)
	if inspectionMode == "all" {
		imageCount = len(store.images)
	}
	planned.Inspection = &inspectionInfo{Mode: inspectionMode, Directory: inspection, Manifest: manifest, ManifestSHA256: hash,
		EncodedSamples: encoded, VisualReview: "pending", UniqueImages: imageCount}
	if err := bounded.Err(); err != nil {
		return empty, nil, err
	}
	planned.DurationSeconds = mp4Info.DurationSeconds
	if !slices.Contains(formats, "mp4") {
		if err := os.Remove(mp4); err != nil {
			return empty, nil, fmt.Errorf("remove known session MP4 intermediate: %w", err)
		}
		planned.DurationSeconds = planned.MediaDetails["gif"].DurationSeconds
	}
	planned.Status = "complete"
	return planned, timeline, nil
}

func sessionProgressFilter(plan rendering) string {
	return fmt.Sprintf("[0:v]drawbox=x=0:y=ih-4:w=iw:h=4:color=0x30363d:t=fill[base];"+
		"color=c=0x79c0ff:s=%dx4:r=%d:d=%.9f[progress];"+
		"[base][progress]overlay=x='-overlay_w+overlay_w*min(1,max(0,(n-1)/%d))':"+
		"y=main_h-overlay_h:eval=frame:shortest=1:format=auto[video]",
		plan.Width, plan.FPS, plan.DurationSeconds, max(1, plan.Frames-1))
}
