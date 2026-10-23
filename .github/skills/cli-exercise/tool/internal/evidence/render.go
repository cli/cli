package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/cli-exercise/internal/cliutil"
	"github.com/cli/cli/v2/cli-exercise/internal/fontutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

const gifFilter = "[0:v]split[a][b];[a]palettegen=stats_mode=single[p];[b][p]paletteuse=new=1"

type mediaInfo struct {
	SHA256          string  `json:"sha256"`
	Bytes           int64   `json:"bytes"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	Frames          int     `json:"frames"`
	DurationSeconds float64 `json:"durationSeconds"`
	FrameRate       string  `json:"frameRate"`
}

type inspectionInfo struct {
	Mode           string              `json:"mode"`
	Directory      string              `json:"directory"`
	Manifest       string              `json:"manifest"`
	ManifestSHA256 string              `json:"manifestSha256"`
	EncodedSamples map[string][]string `json:"encodedSamples"`
	VisualReview   string              `json:"visualReview"`
	UniqueImages   int                 `json:"uniqueImages"`
	RenderedCount  int                 `json:"renderedOccurrences"`
	SourceCount    int                 `json:"sourceOccurrences"`
}

type rendering struct {
	Status                  string               `json:"status"`
	Error                   string               `json:"error,omitempty"`
	Media                   map[string]string    `json:"media"`
	MediaDetails            map[string]mediaInfo `json:"mediaDetails,omitempty"`
	Width                   int                  `json:"width,omitempty"`
	Height                  int                  `json:"height,omitempty"`
	DurationSeconds         float64              `json:"durationSeconds,omitempty"`
	CaptureDurationSeconds  float64              `json:"captureDurationSeconds,omitempty"`
	EndHoldSeconds          *float64             `json:"endHoldSeconds,omitempty"`
	PresentationHoldSeconds float64              `json:"presentationHoldSeconds,omitempty"`
	Frames                  int                  `json:"frames,omitempty"`
	FPS                     int                  `json:"fps,omitempty"`
	Timing                  string               `json:"timing,omitempty"`
	CapturedStates          int                  `json:"capturedStates,omitempty"`
	VideoStates             int                  `json:"videoStates,omitempty"`
	Font                    *fontutil.Info       `json:"font,omitempty"`
	FontStyles              []fontutil.Info      `json:"fontStyles,omitempty"`
	FontFallbacks           []fontutil.Info      `json:"fontFallbacks,omitempty"`
	Inspection              *inspectionInfo      `json:"inspection,omitempty"`
	Warnings                []string             `json:"warnings,omitempty"`
	inspectionFrames        []int
}

type tailBuffer struct{ data []byte }

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	buffer.data = append(buffer.data, data...)
	if len(buffer.data) > 4000 {
		buffer.data = buffer.data[len(buffer.data)-4000:]
	}
	return len(data), nil
}

func rendererEnvironment(directory string, tools recording.Tools) ([]string, error) {
	values, err := cliutil.IsolatedEnvironment(directory, ".")
	if err != nil {
		return nil, err
	}
	if tools.FFmpeg == nil || tools.FFprobe == nil {
		return nil, fmt.Errorf("the renderer requires ffmpeg and ffprobe selections")
	}
	paths := []string{filepath.Dir(tools.FFmpeg.Path), filepath.Dir(tools.FFprobe.Path)}
	paths = append(paths, "/usr/bin", "/bin")
	values["LANG"], values["LC_ALL"] = "C", "C"
	for _, key := range []string{"SystemRoot", "WINDIR"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
			paths = append(paths, filepath.Join(value, "System32"))
		}
	}
	var unique []string
	for _, path := range paths {
		if path != "" && path != "." && !slices.Contains(unique, path) {
			unique = append(unique, path)
		}
	}
	values["PATH"] = strings.Join(unique, string(os.PathListSeparator))
	return cliutil.EnvironmentList(values), nil
}

func runCommand(ctx context.Context, arguments, environment []string, directory string, stdout io.Writer) error {
	command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	command.Env, command.Dir, command.Stdout = environment, directory, stdout
	diagnostic := &tailBuffer{}
	command.Stderr = diagnostic
	command.WaitDelay = 5 * time.Second
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w: %s", filepath.Base(arguments[0]), err, strings.TrimSpace(string(diagnostic.data)))
	}
	return nil
}

func encodingArguments(ffmpeg, destination string, width, height, fps int, kind string) ([]string, error) {
	arguments := []string{
		ffmpeg, "-v", "error", "-nostdin", "-y", "-f", "rawvideo", "-pix_fmt", "rgb24",
		"-s", fmt.Sprintf("%dx%d", width, height), "-r", strconv.Itoa(fps), "-i", "-", "-an",
	}
	switch kind {
	case "mp4":
		arguments = append(arguments, "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart")
	case "gif":
		arguments = append(arguments, "-filter_complex", gifFilter, "-c:v", "gif")
	default:
		return nil, fmt.Errorf("unsupported media format %q", kind)
	}
	return append(arguments, destination), nil
}

func savePNG(path string, canvas image.Image) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return png.Encode(file, canvas)
}

func rgbPixels(canvas *image.RGBA) []byte {
	pixels := make([]byte, canvas.Bounds().Dx()*canvas.Bounds().Dy()*3)
	offset := 0
	for y := canvas.Bounds().Min.Y; y < canvas.Bounds().Max.Y; y++ {
		row := canvas.Pix[canvas.PixOffset(canvas.Bounds().Min.X, y):]
		for x := range canvas.Bounds().Dx() {
			copy(pixels[offset:offset+3], row[4*x:4*x+3])
			offset += 3
		}
	}
	return pixels
}

func inspectMedia(ctx context.Context, filename, kind string, tools recording.Tools, environment []string, width, height, count int) (mediaInfo, error) {
	var info mediaInfo
	if err := runCommand(ctx, []string{tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", filename, "-f", "null", "-"},
		environment, filepath.Dir(filename), io.Discard); err != nil {
		return info, err
	}
	var output bytes.Buffer
	if err := runCommand(ctx, []string{
		tools.FFprobe.Path, "-v", "error", "-select_streams", "v:0", "-count_frames", "-show_entries",
		"stream=codec_name,width,height,nb_read_frames,r_frame_rate:format=duration,size", "-of", "json", filename,
	}, environment, filepath.Dir(filename), &output); err != nil {
		return info, err
	}
	var observed struct {
		Streams []struct {
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			ReadFrames string `json:"nb_read_frames"`
			FrameRate  string `json:"r_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output.Bytes(), &observed); err != nil || len(observed.Streams) != 1 {
		return info, fmt.Errorf("ffprobe returned an invalid video stream description")
	}
	stream := observed.Streams[0]
	frames, err := strconv.Atoi(stream.ReadFrames)
	if err != nil {
		return info, fmt.Errorf("ffprobe did not report a valid decoded frame count")
	}
	codec := "gif"
	if kind == "mp4" {
		codec = "h264"
	}
	if frames != count || stream.Width != width || stream.Height != height || stream.CodecName != codec {
		return info, fmt.Errorf("encoded %s dimensions, codec, or frame count differ from the timeline", kind)
	}
	duration, err := strconv.ParseFloat(observed.Format.Duration, 64)
	if err != nil || !finite(duration) || duration <= 0 {
		return info, fmt.Errorf("ffprobe did not report a valid media duration")
	}
	hash, err := cliutil.SHA256File(filename)
	if err != nil {
		return info, err
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return info, err
	}
	return mediaInfo{
		SHA256: hash, Bytes: stat.Size(), Width: width, Height: height, Frames: frames,
		DurationSeconds: duration, FrameRate: stream.FrameRate,
	}, nil
}

func decodedSamples(ctx context.Context, filename, kind string, tools recording.Tools, environment []string, width, height, count int, inspection string) ([]string, error) {
	frames := slices.Compact([]int{0, count / 2, count - 1})
	selectors := make([]string, 0, len(frames))
	for _, index := range frames {
		selectors = append(selectors, fmt.Sprintf(`eq(n\,%d)`, index))
	}
	var output bytes.Buffer
	if err := runCommand(ctx, []string{
		tools.FFmpeg.Path, "-v", "error", "-nostdin", "-i", filename, "-vf", "select=" + strings.Join(selectors, "+"),
		"-fps_mode", "passthrough", "-an", "-pix_fmt", "rgb24", "-f", "rawvideo", "-",
	}, environment, filepath.Dir(filename), &output); err != nil {
		return nil, err
	}
	frameBytes := width * height * 3
	if output.Len() != len(frames)*frameBytes {
		return nil, fmt.Errorf("decoded %s inspection frames differ from the expected geometry or count", kind)
	}
	directory := filepath.Join(inspection, "encoded", kind)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(frames))
	for offset, index := range frames {
		canvas := image.NewRGBA(image.Rect(0, 0, width, height))
		pixels := output.Bytes()[offset*frameBytes : (offset+1)*frameBytes]
		for pixel := range width * height {
			copy(canvas.Pix[pixel*4:pixel*4+3], pixels[pixel*3:pixel*3+3])
			canvas.Pix[pixel*4+3] = 255
		}
		name := fmt.Sprintf("frame-%06d.png", index)
		if err := savePNG(filepath.Join(directory, name), canvas); err != nil {
			return nil, err
		}
		paths = append(paths, filepath.ToSlash(filepath.Join("encoded", kind, name)))
	}
	return paths, nil
}

func render(ctx context.Context, contract contract, result result, states []state, events []event, receipt recording.Receipt, directory, inspectionMode string) (_ rendering, err error) {
	empty := rendering{}
	if len(contract.Output.Formats) == 0 {
		return empty, fmt.Errorf("output formats must be gif, mp4, or both")
	}
	for _, kind := range contract.Output.Formats {
		if !slices.Contains([]string{"gif", "mp4"}, kind) || !receipt.Checks.Formats[kind] {
			return empty, fmt.Errorf("ready receipt does not confirm every requested output format")
		}
	}
	if contract.Terminal.FPS < 1 || contract.Terminal.FPS > 60 {
		return empty, fmt.Errorf("frame rate must be a whole number from 1 through 60")
	}
	captions := contract.Output.Captions == nil || *contract.Output.Captions
	if contract.Output.Timing == "condensed" && !captions {
		return empty, fmt.Errorf("unannotated output preserves real timing; use realtime instead of condensed")
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		selected := receipt.Tools.Executable(name)
		if selected == nil || !filepath.IsAbs(selected.Path) {
			return empty, fmt.Errorf("receipt must select an absolute %s executable", name)
		}
		if selected.SHA256 != "" {
			hash, err := cliutil.SHA256File(selected.Path)
			if err != nil || hash != selected.SHA256 {
				return empty, fmt.Errorf("selected %s no longer matches the ready receipt", name)
			}
		}
	}
	raster, err := newRasterizer(contract.Terminal)
	if err != nil {
		return empty, err
	}
	defer func() { err = errors.Join(err, raster.fonts.Close()) }()
	if contract.Chapter != nil {
		events = chapterEvents(*contract.Chapter, events)
	}
	plan, err := framePlan(states, events, result.DurationSeconds, contract.Terminal.FPS, contract.Output.Timing)
	if err != nil {
		return empty, err
	}
	width, terminalHeight, count := 0, 0, 0
	videoStates := map[int]bool{}
	for _, state := range states {
		width = max(width, state.Data.Columns*raster.fonts.CellWidth)
		terminalHeight = max(terminalHeight, state.Data.Rows*raster.fonts.CellHeight)
	}
	commandLines, err := raster.commandLines(contract, width)
	if err != nil {
		return empty, err
	}
	terminalHeight += len(commandLines) * raster.fonts.CellHeight
	for _, frame := range plan {
		count += frame.Repeats
		videoStates[frame.State] = true
	}
	additionalFrames := 0
	if contract.Chapter != nil && contract.Output.Timing == "condensed" {
		minimum := contract.Chapter.MinimumDurationSeconds
		if !finite(minimum) || minimum < 0 || minimum > 60 {
			return empty, fmt.Errorf("chapter reading duration must be between 0 and 60 seconds")
		}
		additionalFrames = max(0, int(math.Ceil(minimum*float64(contract.Terminal.FPS)))-count)
		plan[len(plan)-1].Repeats += additionalFrames
		count += additionalFrames
	}
	height := terminalHeight
	if captions {
		height += 4 * raster.fonts.CellHeight
	}
	width += width % 2
	height += height % 2
	mediaWidth := width + 2*presentationMargin
	mediaHeight := height + 2*presentationMargin
	if width <= 0 || height <= 0 || int64(mediaWidth)*int64(mediaHeight) > 64_000_000 {
		return empty, fmt.Errorf("recorded geometry exceeds the bounded raster dimensions")
	}
	environment, err := rendererEnvironment(directory, receipt.Tools)
	if err != nil {
		return empty, err
	}
	inspection := filepath.Join(directory, "inspection")
	if err := os.Mkdir(inspection, 0o700); err != nil {
		return empty, err
	}
	store, err := newImageStore(inspection)
	if err != nil {
		return empty, err
	}
	renderedOccurrences := []renderedOccurrence{}
	sourceOccurrences := []sourceOccurrence{}
	primaryKind := "gif"
	if slices.Contains(contract.Output.Formats, "mp4") {
		primaryKind = "mp4"
	}
	primary := filepath.Join(directory, "terminal."+primaryKind)
	arguments, err := encodingArguments(receipt.Tools.FFmpeg.Path, primary, mediaWidth, mediaHeight, contract.Terminal.FPS, primaryKind)
	if err != nil {
		return empty, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Duration(max(120, min(1800, result.DurationSeconds*2+30)))*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, arguments[0], arguments[1:]...)
	command.Env, command.Dir = environment, directory
	diagnostics := &tailBuffer{}
	command.Stderr = diagnostics
	command.WaitDelay = 5 * time.Second
	input, err := command.StdinPipe()
	if err != nil {
		return empty, err
	}
	if err := command.Start(); err != nil {
		return empty, errors.Join(err, input.Close())
	}
	produce := func() error {
		var top *image.RGBA
		var canvas *image.RGBA
		var pixels []byte
		previousState, number := -1, 0
		var previousNote *event
		imagePath := ""
		seen := map[int]bool{}
		for _, entry := range plan {
			if err := bounded.Err(); err != nil {
				return err
			}
			changed := entry.State != previousState || entry.Note != previousNote || canvas == nil
			if entry.State != previousState {
				top, err = raster.draw(states[entry.State].Data)
				if err != nil {
					return err
				}
				previousState = entry.State
			}
			if changed {
				canvas, err = raster.compose(top, contract, entry, width, height, terminalHeight)
				if err != nil {
					return err
				}
				pixels = rgbPixels(canvas)
				previousNote = entry.Note
				imagePath = ""
			}
			if inspectionMode == "all" || number == 0 || number+entry.Repeats == count || !seen[number/contract.Terminal.FPS] {
				if imagePath == "" {
					imagePath, err = store.save(canvas)
					if err != nil {
						return err
					}
				}
				var noteTime *float64
				if entry.Note != nil {
					noteTime = entry.Note.Time
				}
				renderedOccurrences = append(renderedOccurrences, renderedOccurrence{
					Image: imagePath, FirstFrame: number, LastFrame: number + entry.Repeats - 1,
					CaptureTimeSeconds: entry.Time, SourceState: entry.State,
					Revision: states[entry.State].Revision, NoteTimeSeconds: noteTime,
				})
				seen[number/contract.Terminal.FPS] = true
			}
			for range entry.Repeats {
				if _, err := input.Write(pixels); err != nil {
					return err
				}
				number++
			}
		}
		return nil
	}
	produceErr := produce()
	closeErr := input.Close()
	if produceErr != nil {
		cancel()
	}
	waitErr := command.Wait()
	if err := errors.Join(produceErr, closeErr, waitErr); err != nil {
		return empty, fmt.Errorf("video encoding failed: %w: %s", err, strings.TrimSpace(string(diagnostics.data)))
	}
	if inspectionMode == "all" {
		for index, state := range states {
			canvas, err := raster.draw(state.Data)
			if err != nil {
				return empty, err
			}
			path, err := store.save(canvas)
			if err != nil {
				return empty, err
			}
			sourceOccurrences = append(sourceOccurrences, sourceOccurrence{
				Image: path, State: index, Time: *state.Time, Revision: state.Revision, AlternateScreen: state.AlternateScreen,
			})
		}
	}
	media := map[string]string{primaryKind: primary}
	if primaryKind != "gif" && slices.Contains(contract.Output.Formats, "gif") {
		gif := filepath.Join(directory, "terminal.gif")
		if err := runCommand(bounded, []string{
			receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-y", "-i", primary,
			"-filter_complex", gifFilter, "-c:v", "gif", gif,
		}, environment, directory, io.Discard); err != nil {
			return empty, err
		}
		media["gif"] = gif
	}
	details := map[string]mediaInfo{}
	encoded := map[string][]string{}
	for _, kind := range []string{"gif", "mp4"} {
		filename, exists := media[kind]
		if !exists {
			continue
		}
		details[kind], err = inspectMedia(bounded, filename, kind, receipt.Tools, environment, mediaWidth, mediaHeight, count)
		if err != nil {
			return empty, err
		}
		encoded[kind], err = decodedSamples(bounded, filename, kind, receipt.Tools, environment, mediaWidth, mediaHeight, count, inspection)
		if err != nil {
			return empty, err
		}
	}
	images := map[string]string{}
	if err := filepath.WalkDir(inspection, func(path string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() || filepath.Ext(path) != ".png" {
			return nil
		}
		hash, err := cliutil.SHA256File(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(inspection, path)
		if err != nil {
			return err
		}
		images[filepath.ToSlash(relative)] = hash
		return nil
	}); err != nil {
		return empty, err
	}
	manifest := filepath.Join(inspection, "manifest.json")
	if err := cliutil.WriteJSON(manifest, map[string]any{
		"frames": images, "media": details, "fps": contract.Terminal.FPS, "timing": contract.Output.Timing,
		"rendered": renderedOccurrences, "sourceStates": sourceOccurrences,
	}); err != nil {
		return empty, err
	}
	hash, err := cliutil.SHA256File(manifest)
	if err != nil {
		return empty, err
	}
	timelineDuration := float64(count) / float64(contract.Terminal.FPS)
	var hold *float64
	if contract.Output.Timing == "realtime" {
		value := max(0, timelineDuration-result.DurationSeconds)
		hold = &value
	}
	var inspectionFrames []int
	if inspectionMode == "all" {
		inspectionFrames = renderedInspectionFrames(renderedOccurrences, count, additionalFrames)
	}
	return rendering{
		Status: "complete", Media: media, MediaDetails: details, Width: mediaWidth, Height: mediaHeight,
		DurationSeconds: timelineDuration, CaptureDurationSeconds: result.DurationSeconds, EndHoldSeconds: hold,
		PresentationHoldSeconds: float64(additionalFrames) / float64(contract.Terminal.FPS),
		Frames:                  count, FPS: contract.Terminal.FPS, Timing: contract.Output.Timing,
		CapturedStates: len(states), VideoStates: len(videoStates), Font: &raster.fonts.Info,
		FontStyles: raster.fonts.Styles, FontFallbacks: raster.fonts.Fallbacks,
		Inspection: &inspectionInfo{
			Mode: inspectionMode, Directory: inspection, Manifest: manifest, ManifestSHA256: hash,
			EncodedSamples: encoded, VisualReview: "pending",
			UniqueImages: len(store.images), RenderedCount: len(renderedOccurrences), SourceCount: len(sourceOccurrences),
		},
		Warnings:         raster.fonts.Warnings(),
		inspectionFrames: inspectionFrames,
	}, nil
}
