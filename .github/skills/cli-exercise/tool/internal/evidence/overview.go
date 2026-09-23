package evidence

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/cli-exercise/internal/fontutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

const overviewPageSeconds = 10

type overviewRow struct {
	result sessionCaseResult
	lines  []string
	height int
}

type overviewLayout struct {
	title  []string
	pages  [][]overviewRow
	top    int
	footer int
}

func fidelityLabel(mode string) string {
	switch mode {
	case "exact":
		return "Exact run"
	case "explore":
		return "Exploration"
	case "mixed":
		return "Mixed run"
	default:
		return "Recorded run"
	}
}

func recordingMode(modes []string) string {
	found := ""
	for _, mode := range modes {
		if !slices.Contains([]string{"exact", "explore", "mixed"}, mode) {
			return "unknown"
		}
		if found != "" && found != mode {
			found = "mixed"
		} else {
			found = mode
		}
	}
	if found == "" {
		return "unknown"
	}
	return found
}

func wrapOverviewText(fonts *fontutil.Set, text string, width, limit int) ([]string, error) {
	lines := []string{}
	for word := range strings.FieldsSeq(text) {
		length, err := fonts.Measure(word)
		if err != nil {
			return nil, err
		}
		if length > float64(width) {
			return nil, fmt.Errorf("overview text does not fit; shorten the title or increase the recording width")
		}
		if len(lines) > 0 {
			candidate := lines[len(lines)-1] + " " + word
			length, err := fonts.Measure(candidate)
			if err != nil {
				return nil, err
			}
			if length <= float64(width) {
				lines[len(lines)-1] = candidate
				continue
			}
		}
		lines = append(lines, word)
	}
	if len(lines) == 0 || len(lines) > limit {
		return nil, fmt.Errorf("overview titles must be brief natural-language descriptions fitting at most %d lines", limit)
	}
	return lines, nil
}

func planOverview(fonts, heading *fontutil.Set, title string, cases []sessionCaseResult, width, height int) (overviewLayout, error) {
	layout := overviewLayout{}
	contentWidth := width - 2*presentationMargin
	contentHeight := height - 2*presentationMargin
	if contentWidth <= 64 || contentHeight <= 0 {
		return layout, fmt.Errorf("overview needs room for the framed presentation")
	}
	var err error
	layout.title, err = wrapOverviewText(heading, title, contentWidth-64, 2)
	if err != nil {
		return layout, err
	}
	layout.top = 40 + fonts.CellHeight + len(layout.title)*heading.CellHeight + 28
	layout.footer = contentHeight - 2*fonts.CellHeight - 28
	available := layout.footer - layout.top
	if available < fonts.CellHeight+20 || len(cases) == 0 {
		return layout, fmt.Errorf("overview needs room for at least one readable result row")
	}
	page, used := []overviewRow{}, 0
	for _, item := range cases {
		lines, err := wrapOverviewText(fonts, item.Title, contentWidth-224, 2)
		if err != nil {
			return layout, fmt.Errorf("case %s: %w", item.ID, err)
		}
		row := overviewRow{result: item, lines: lines, height: len(lines)*fonts.CellHeight + 20}
		if row.height > available {
			return layout, fmt.Errorf("case %s does not fit a readable overview page", item.ID)
		}
		if used+row.height > available {
			layout.pages = append(layout.pages, page)
			page, used = nil, 0
		}
		page, used = append(page, row), used+row.height
	}
	layout.pages = append(layout.pages, page)
	return layout, nil
}

func statusMark(canvas *image.RGBA, x, y, size int, status string, ink color.RGBA) {
	line := func(x1, y1, x2, y2 int) {
		steps := max(max(x2-x1, x1-x2), max(y2-y1, y1-y2))
		for step := 0; step <= steps; step++ {
			px, py := x1, y1
			if steps > 0 {
				px, py = x1+(x2-x1)*step/steps, y1+(y2-y1)*step/steps
			}
			fill(canvas, image.Rect(px, py, px+2, py+2), ink)
		}
	}
	switch status {
	case "passed":
		line(x, y+size/2, x+size/3, y+size)
		line(x+size/3, y+size, x+size, y)
	case "failed":
		line(x, y, x+size, y+size)
		line(x, y+size, x+size, y)
	default:
		line(x, y+size/2, x+size, y+size/2)
	}
}

func drawOverview(raster *rasterizer, heading *fontutil.Set, layout overviewLayout, page, width, height int, mode string) (*image.RGBA, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(canvas, canvas.Bounds(), presentationInk)
	contentWidth := width - 2*presentationMargin
	contentHeight := height - 2*presentationMargin
	panel := image.Rect(presentationMargin-1, presentationMargin-1,
		presentationMargin+contentWidth+1, presentationMargin+contentHeight+1)
	fill(canvas, panel, panelBorderInk)
	fill(canvas, image.Rect(presentationMargin, presentationMargin,
		presentationMargin+contentWidth, presentationMargin+contentHeight), raster.background)
	if err := raster.fonts.DrawText(canvas, presentationMargin+32,
		presentationMargin+24+raster.fonts.Ascent, fidelityLabel(mode), runningInk); err != nil {
		return nil, err
	}
	y := presentationMargin + 40 + raster.fonts.CellHeight
	for _, line := range layout.title {
		if err := heading.DrawText(canvas, presentationMargin+32, y+heading.Ascent, line, raster.foreground); err != nil {
			return nil, err
		}
		y += heading.CellHeight
	}
	y = presentationMargin + layout.top
	border := color.RGBA{0x30, 0x36, 0x3d, 0xff}
	for _, row := range layout.pages[page] {
		label, ink := outcomeAppearance(row.result.Status)
		statusMark(canvas, presentationMargin+34, y+5, max(10, raster.fonts.CellHeight-10), row.result.Status, ink)
		for index, line := range row.lines {
			if err := raster.fonts.DrawText(canvas, presentationMargin+64,
				y+index*raster.fonts.CellHeight+raster.fonts.Ascent, line, raster.foreground); err != nil {
				return nil, err
			}
		}
		labelWidth, err := raster.fonts.Measure(label)
		if err != nil {
			return nil, err
		}
		if err := raster.fonts.DrawText(canvas, presentationMargin+contentWidth-32-int(labelWidth),
			y+raster.fonts.Ascent, label, ink); err != nil {
			return nil, err
		}
		y += row.height
		fill(canvas, image.Rect(presentationMargin+32, y-8,
			presentationMargin+contentWidth-32, y-7), border)
	}
	footer := "Recorded outcomes"
	if len(layout.pages) > 1 {
		footer += fmt.Sprintf(" | Page %d of %d", page+1, len(layout.pages))
	}
	if err := raster.fonts.DrawText(canvas, presentationMargin+32,
		presentationMargin+contentHeight-24-raster.fonts.CellHeight+raster.fonts.Ascent, footer, mutedInk); err != nil {
		return nil, err
	}
	return canvas, nil
}

func renderOverview(ctx context.Context, directory string, receipt recording.Receipt, config terminalConfig,
	title, mode string, cases []sessionCaseResult, width, height, fps int) (_ []sessionChapter, _ *sessionOverview, err error) {
	if fps < 1 || fps > 60 || width <= 0 || height <= 0 || width%2 != 0 || height%2 != 0 || width > 64_000_000/height {
		return nil, nil, fmt.Errorf("overview requires bounded even geometry and a supported frame rate")
	}
	raster, err := newRasterizer(config, receipt)
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, raster.fonts.Close()) }()
	heading, err := fontutil.Open(raster.fonts.Info.Path, config.FontSize*1.4, config.FontFallbacks)
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, heading.Close()) }()
	layout, err := planOverview(raster.fonts, heading, title, cases, width, height)
	if err != nil {
		return nil, nil, err
	}
	if len(layout.pages) > 1_000_000/(overviewPageSeconds*fps) {
		return nil, nil, fmt.Errorf("overview exceeds the bounded video frame count")
	}
	environment, err := rendererEnvironment(directory, receipt.Tools)
	if err != nil {
		return nil, nil, err
	}
	chapters := make([]sessionChapter, 0, len(layout.pages))
	for page := range layout.pages {
		canvas, err := drawOverview(raster, heading, layout, page, width, height, mode)
		if err != nil {
			return nil, nil, err
		}
		imagePath := filepath.Join(directory, fmt.Sprintf("overview-%03d.png", page+1))
		if err := savePNG(imagePath, canvas); err != nil {
			return nil, nil, err
		}
		videoPath := filepath.Join(directory, fmt.Sprintf("overview-%03d.mp4", page+1))
		if err := runCommand(ctx, []string{
			receipt.Tools.FFmpeg.Path, "-v", "error", "-nostdin", "-n", "-loop", "1", "-framerate", strconv.Itoa(fps),
			"-i", imagePath, "-frames:v", strconv.Itoa(overviewPageSeconds * fps), "-an",
			"-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", "-movflags", "+faststart", videoPath,
		}, environment, directory, io.Discard); err != nil {
			return nil, nil, err
		}
		label := fidelityLabel(mode) + ": " + title
		if len(layout.pages) > 1 {
			label += fmt.Sprintf(" (%d/%d)", page+1, len(layout.pages))
		}
		chapters = append(chapters, sessionChapter{Kind: "overview", Title: label,
			Evidence: report{Rendering: rendering{
				Status: "complete", Media: map[string]string{"mp4": videoPath},
				Width: width, Height: height, Frames: overviewPageSeconds * fps, FPS: fps,
				DurationSeconds: overviewPageSeconds,
				Font:            &raster.fonts.Info, FontFallbacks: raster.fonts.Fallbacks,
			}}})
	}
	return chapters, &sessionOverview{Mode: mode, Title: title, Pages: len(layout.pages),
		SecondsPerPage: overviewPageSeconds, DurationSeconds: float64(overviewPageSeconds * len(layout.pages))}, nil
}
