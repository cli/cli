package evidence

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"
	"unicode"

	"github.com/cli/cli/v2/cli-exercise/internal/fontutil"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

type rasterizer struct {
	fonts      *fontutil.Set
	background color.RGBA
	foreground color.RGBA
}

var (
	preparationInk  = color.RGBA{0xd2, 0xa8, 0xff, 0xff}
	runningInk      = color.RGBA{0x79, 0xc0, 0xff, 0xff}
	passedInk       = color.RGBA{0x7e, 0xe7, 0x87, 0xff}
	failedInk       = color.RGBA{0xff, 0x7b, 0x72, 0xff}
	blockedInk      = color.RGBA{0xe3, 0xb3, 0x41, 0xff}
	mutedInk        = color.RGBA{0x91, 0x98, 0xa1, 0xff}
	presentationInk = color.RGBA{0x01, 0x04, 0x09, 0xff}
	panelBorderInk  = color.RGBA{0x30, 0x36, 0x3d, 0xff}
)

const presentationMargin = 48

func outcomeAppearance(status string) (string, color.RGBA) {
	switch status {
	case "passed":
		return "Passed", passedInk
	case "failed":
		return "Failed", failedInk
	case "blocked":
		return "Blocked", blockedInk
	case "observed":
		return "Observed", mutedInk
	default:
		return "Unverified", mutedInk
	}
}

func phaseAppearance(phase, status string) (string, color.RGBA) {
	switch phase {
	case "setup":
		return "Preparation", preparationInk
	case "exercise":
		return "Running", runningInk
	case "validation":
		label, ink := outcomeAppearance(status)
		return "Validation - " + label, ink
	case "cleanup":
		return "Cleanup", mutedInk
	default:
		return "Recorded", mutedInk
	}
}

func (raster *rasterizer) commandLines(contract contract, width int) ([]string, error) {
	if contract.Output.Captions != nil && !*contract.Output.Captions {
		return nil, nil
	}
	command := ""
	if contract.Chapter != nil {
		command = contract.Chapter.Command
	} else if contract.Command.Executable != "" {
		command = displayCommand(contract.Command.Executable, contract.Command.Args)
	}
	if command == "" {
		return nil, nil
	}
	lines := []string{""}
	used := 0.0
	for _, r := range "$ " + command {
		if r == '\n' {
			lines, used = append(lines, ""), 0
			continue
		}
		advance, err := raster.fonts.Measure(string(r))
		if err != nil {
			return nil, err
		}
		if advance > float64(width) {
			return nil, fmt.Errorf("recorded command glyph does not fit the terminal width")
		}
		if used+advance > float64(width) {
			lines, used = append(lines, ""), 0
		}
		lines[len(lines)-1] += string(r)
		used += advance
		if len(lines) > 1000 {
			return nil, fmt.Errorf("recorded command exceeds the bounded terminal header")
		}
	}
	return lines, nil
}

func parseColor(value string) (color.RGBA, error) {
	if len(value) != 7 || value[0] != '#' {
		return color.RGBA{}, fmt.Errorf("terminal colors must be #RRGGBB values")
	}
	rgb, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid terminal color %q", value)
	}
	return color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}, nil
}

func newRasterizer(config terminalConfig) (*rasterizer, error) {
	background, err := parseColor(config.Background)
	if err != nil {
		return nil, err
	}
	foreground, err := parseColor(config.Foreground)
	if err != nil {
		return nil, err
	}
	path := config.FontPath
	if path == "" {
		return nil, fmt.Errorf("rendering requires an explicit font selection")
	}
	fonts, err := fontutil.Open(path, config.FontSize, config.FontFallbacks)
	if err != nil {
		return nil, err
	}
	return &rasterizer{fonts: fonts, background: background, foreground: foreground}, nil
}

func fill(canvas draw.Image, rectangle image.Rectangle, ink color.Color) {
	draw.Draw(canvas, rectangle, image.NewUniform(ink), image.Point{}, draw.Src)
}

func (raster *rasterizer) draw(data recording.TerminalData) (*image.RGBA, error) {
	if data.Columns < 1 || data.Rows < 1 || data.Columns > 1000 || data.Rows > 1000 || data.Lines == nil {
		return nil, fmt.Errorf("recorded terminal has invalid dimensions or lines")
	}
	width, height := data.Columns*raster.fonts.CellWidth, data.Rows*raster.fonts.CellHeight
	if int64(width)*int64(height) > 64_000_000 {
		return nil, fmt.Errorf("terminal raster exceeds the 64-megapixel frame limit")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(canvas, canvas.Bounds(), raster.background)
	lines := data.Lines[max(0, len(data.Lines)-data.Rows):]
	for row, line := range lines {
		x, y := 0, row*raster.fonts.CellHeight
		for _, span := range line.Spans {
			if span.Width == nil || *span.Width < 0 || *span.Width > data.Columns {
				return nil, fmt.Errorf("recorded terminal span has an invalid cell width")
			}
			foreground, background := raster.foreground, raster.background
			var err error
			if span.FG != "" {
				foreground, err = parseColor(span.FG)
				if err != nil {
					return nil, err
				}
			}
			if span.BG != "" {
				background, err = parseColor(span.BG)
				if err != nil {
					return nil, err
				}
			}
			if span.Flags&16 != 0 {
				foreground, background = background, foreground
			}
			if span.Flags&32 != 0 {
				foreground.R = uint8((int(foreground.R) + int(background.R)) / 2)
				foreground.G = uint8((int(foreground.G) + int(background.G)) / 2)
				foreground.B = uint8((int(foreground.B) + int(background.B)) / 2)
			}
			spanWidth := *span.Width * raster.fonts.CellWidth
			if x+spanWidth > width {
				return nil, fmt.Errorf("recorded terminal spans exceed the visible columns")
			}
			fill(canvas, image.Rect(x, y, x+spanWidth, y+raster.fonts.CellHeight), background)
			if err := raster.fonts.DrawCells(canvas, x, y+raster.fonts.Ascent, span.Text, span.Flags, foreground); err != nil {
				return nil, err
			}
			if span.Flags&4 != 0 {
				fill(canvas, image.Rect(x, y+raster.fonts.Ascent+1, x+spanWidth, y+raster.fonts.Ascent+2), foreground)
			}
			if span.Flags&8 != 0 {
				fill(canvas, image.Rect(x, y+raster.fonts.Ascent/2, x+spanWidth, y+raster.fonts.Ascent/2+1), foreground)
			}
			x += spanWidth
		}
	}
	if data.CursorVisible != nil && *data.CursorVisible {
		if len(data.Cursor) != 2 {
			return nil, fmt.Errorf("recorded terminal cursor must contain column and row")
		}
		column, row := data.Cursor[0], data.Cursor[1]
		if column >= 0 && column < data.Columns && row >= 0 && row < data.Rows {
			x, y := column*raster.fonts.CellWidth, row*raster.fonts.CellHeight
			cell := image.Rect(x, y, x+raster.fonts.CellWidth, y+raster.fonts.CellHeight)
			switch data.CursorStyle {
			case "bar":
				fill(canvas, image.Rect(x, y, x+2, cell.Max.Y), raster.foreground)
			case "underline":
				fill(canvas, image.Rect(x, cell.Max.Y-2, cell.Max.X, cell.Max.Y), raster.foreground)
			case "block_hollow":
				fill(canvas, image.Rect(x, y, cell.Max.X, y+1), raster.foreground)
				fill(canvas, image.Rect(x, cell.Max.Y-1, cell.Max.X, cell.Max.Y), raster.foreground)
				fill(canvas, image.Rect(x, y, x+1, cell.Max.Y), raster.foreground)
				fill(canvas, image.Rect(cell.Max.X-1, y, cell.Max.X, cell.Max.Y), raster.foreground)
			case "", "default", "block":
				fill(canvas, cell, raster.foreground)
				if row < len(lines) {
					position := 0
					for _, span := range lines[row].Spans {
						for _, character := range span.Text {
							if position == column && !unicode.IsSpace(character) {
								if err := raster.fonts.DrawCells(canvas, x, y+raster.fonts.Ascent,
									string(character), span.Flags, raster.background); err != nil {
									return nil, err
								}
							}
							position += fontutil.Cells(character)
						}
					}
				}
			default:
				return nil, fmt.Errorf("unsupported recorded cursor style %q", data.CursorStyle)
			}
		}
	}
	return canvas, nil
}

func (raster *rasterizer) compose(top *image.RGBA, contract contract, entry frame, width, height, terminalHeight int) (*image.RGBA, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, width+2*presentationMargin, height+2*presentationMargin))
	fill(canvas, canvas.Bounds(), presentationInk)
	panel := image.Rect(presentationMargin-1, presentationMargin-1,
		presentationMargin+width+1, presentationMargin+height+1)
	fill(canvas, panel, panelBorderInk)
	fill(canvas, image.Rect(presentationMargin, presentationMargin,
		presentationMargin+width, presentationMargin+height), raster.background)
	command, err := raster.commandLines(contract, width)
	if err != nil {
		return nil, err
	}
	offset := len(command) * raster.fonts.CellHeight
	// The invocation is recorded launch metadata, not invented terminal keystrokes.
	for row, line := range command {
		if err := raster.fonts.DrawText(canvas, presentationMargin,
			presentationMargin+row*raster.fonts.CellHeight+raster.fonts.Ascent, line, raster.foreground); err != nil {
			return nil, err
		}
	}
	draw.Draw(canvas, top.Bounds().Add(image.Pt(presentationMargin, presentationMargin+offset)), top, image.Point{}, draw.Src)
	if contract.Output.Captions != nil && !*contract.Output.Captions {
		return canvas, nil
	}
	border, _ := parseColor("#3d444d")
	accent := runningInk
	if contract.Chapter != nil {
		_, accent = phaseAppearance(contract.Chapter.Phase, contract.Chapter.Status)
	}
	terminalBottom := presentationMargin + terminalHeight
	fill(canvas, image.Rect(presentationMargin, terminalBottom, presentationMargin+width, terminalBottom+1), border)
	fill(canvas, image.Rect(presentationMargin, terminalBottom, presentationMargin+width, terminalBottom+3), accent)
	chapter, text := contract.CaseID, contract.Goal
	if entry.Note != nil {
		if entry.Note.Chapter != nil {
			chapter = *entry.Note.Chapter
		}
		text = *entry.Note.Text
	}
	available := float64(width - 24)
	length, err := raster.fonts.Measure(chapter)
	if err != nil {
		return nil, err
	}
	if length > available {
		return nil, fmt.Errorf("annotation chapter does not fit; revise it or the geometry")
	}
	var lines []string
	for word := range strings.FieldsSeq(text) {
		length, err := raster.fonts.Measure(word)
		if err != nil {
			return nil, err
		}
		if length > available {
			return nil, fmt.Errorf("annotation word does not fit; revise it or the geometry")
		}
		if len(lines) > 0 {
			candidate := lines[len(lines)-1] + " " + word
			length, err := raster.fonts.Measure(candidate)
			if err != nil {
				return nil, err
			}
			if length <= available {
				lines[len(lines)-1] = candidate
				continue
			}
		}
		lines = append(lines, word)
	}
	if len(lines) > 2 {
		return nil, fmt.Errorf("annotation exceeds two lines; revise it rather than truncate it")
	}
	if err := raster.fonts.DrawText(canvas, presentationMargin+12, terminalBottom+8+raster.fonts.Ascent, chapter, accent); err != nil {
		return nil, err
	}
	for index, line := range lines {
		baseline := terminalBottom + raster.fonts.CellHeight*(index+1) + 8 + raster.fonts.Ascent
		if err := raster.fonts.DrawText(canvas, presentationMargin+12, baseline, line, raster.foreground); err != nil {
			return nil, err
		}
	}
	return canvas, nil
}
