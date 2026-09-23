// Package fontutil loads explicit terminal fonts without platform font services.
package fontutil

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/width"
)

// Info identifies the actual font bytes used by a capability probe or rendering.
type Info struct {
	Path   string `json:"path"`
	Family string `json:"family"`
	SHA256 string `json:"sha256"`
}

type face struct {
	source *sfnt.Font
	font   font.Face
}

// Set holds explicit font faces and the terminal's fixed cell metrics.
type Set struct {
	Info       Info
	Fallbacks  []Info
	CellWidth  int
	CellHeight int
	Ascent     int
	styles     map[int]*face
	fallbacks  []*face
	faces      []*face
	warnings   map[string]struct{}
}

func load(filename string, size float64) ([]*face, []string, Info, error) {
	var info Info
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, nil, info, err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, nil, info, fmt.Errorf("open selected font: %w", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, nil, info, err
	}
	const limit = 64 << 20
	if !stat.Mode().IsRegular() || stat.Size() > limit {
		return nil, nil, info, fmt.Errorf("selected font must be a regular file no larger than 64 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return nil, nil, info, fmt.Errorf("read selected font: %w", errors.Join(err, fmt.Errorf("font input is unavailable or too large")))
	}
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, nil, info, fmt.Errorf("parse selected OpenType font: %w", err)
	}
	if collection.NumFonts() < 1 || collection.NumFonts() > 64 {
		return nil, nil, info, fmt.Errorf("selected font collection has an unsupported face count")
	}
	hash := sha256.Sum256(data)
	info = Info{Path: absolute, SHA256: hex.EncodeToString(hash[:])}
	var faces []*face
	var styles []string
	for index := range collection.NumFonts() {
		source, err := collection.Font(index)
		if err != nil {
			return nil, nil, info, err
		}
		family, err := source.Name(nil, sfnt.NameIDFamily)
		if err != nil || family == "" {
			return nil, nil, info, fmt.Errorf("selected font has no readable family name")
		}
		style, err := source.Name(nil, sfnt.NameIDSubfamily)
		if err != nil {
			return nil, nil, info, fmt.Errorf("read font style: %w", err)
		}
		if index == 0 {
			info.Family = family
		}
		raster, err := opentype.NewFace(source, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return nil, nil, info, err
		}
		faces = append(faces, &face{source: source, font: raster})
		styles = append(styles, style)
	}
	return faces, styles, info, nil
}

func styleFlags(style string) int {
	style = strings.ToLower(style)
	flags := 0
	if strings.Contains(style, "bold") {
		flags |= 1
	}
	if strings.Contains(style, "italic") || strings.Contains(style, "oblique") {
		flags |= 2
	}
	return flags
}

// Open resolves the selected collection, matching styles, and explicit fallback files.
func Open(filename string, size float64, fallbacks []string) (_ *Set, err error) {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) || size > 512 {
		return nil, fmt.Errorf("font size must be finite and between 0 and 512 points")
	}
	faces, styles, info, err := load(filename, size)
	if err != nil {
		return nil, err
	}
	set := &Set{Info: info, styles: map[int]*face{0: faces[0]}, faces: faces, warnings: map[string]struct{}{}}
	defer func() {
		if err != nil {
			err = errors.Join(err, set.Close())
		}
	}()
	for index, style := range styles {
		if flags := styleFlags(style); flags != 0 {
			set.styles[flags] = faces[index]
		}
	}
	extension := filepath.Ext(filename)
	family := strings.TrimSuffix(filepath.Base(filename), extension)
	family = regexp.MustCompile(`(?i)[-_ ]?(regular|book|roman|normal)$`).ReplaceAllString(family, "")
	candidates := map[int][]string{
		1: {family + "-Bold" + extension},
		2: {family + "-Italic" + extension, family + "-Oblique" + extension},
		3: {family + "-BoldItalic" + extension, family + "-BoldOblique" + extension},
	}
	if strings.EqualFold(filepath.Base(filename), "consola.ttf") {
		candidates = map[int][]string{1: {"consolab.ttf"}, 2: {"consolai.ttf"}, 3: {"consolaz.ttf"}}
	}
	for _, flags := range []int{1, 2, 3} {
		for _, candidate := range candidates[flags] {
			if set.styles[flags] != nil {
				break
			}
			path := filepath.Join(filepath.Dir(filename), candidate)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, err
			}
			extra, _, _, err := load(path, size)
			if err != nil {
				return nil, err
			}
			set.faces = append(set.faces, extra...)
			set.styles[flags] = extra[0]
		}
	}
	for _, path := range fallbacks {
		extra, _, info, err := load(path, size)
		if err != nil {
			return nil, fmt.Errorf("load explicit fallback: %w", err)
		}
		set.faces = append(set.faces, extra...)
		set.fallbacks = append(set.fallbacks, extra[0])
		set.Fallbacks = append(set.Fallbacks, info)
	}
	base := set.styles[0]
	scale := fixed.Int26_6(math.Round(size * 64))
	metrics, err := base.source.Metrics(nil, scale, font.HintingNone)
	if err != nil {
		return nil, fmt.Errorf("read font metrics: %w", err)
	}
	advance, ok := base.font.GlyphAdvance('M')
	if !ok || advance <= 0 {
		return nil, fmt.Errorf("selected font has no usable cell advance")
	}
	for character := rune(32); character < 127; character++ {
		index, indexErr := base.source.GlyphIndex(nil, character)
		got, ok := base.font.GlyphAdvance(character)
		if indexErr != nil || index == 0 || !ok || got != advance {
			return nil, fmt.Errorf("selected font is not monospaced across printable ASCII")
		}
	}
	set.CellWidth = advance.Ceil()
	set.Ascent = metrics.Ascent.Ceil()
	set.CellHeight = metrics.Ascent.Ceil() + metrics.Descent.Ceil() + 2
	if set.Ascent <= 0 || set.CellHeight <= 2 {
		return nil, fmt.Errorf("selected font has invalid vertical metrics")
	}
	return set, nil
}

// Close releases every resolved face.
func (set *Set) Close() error {
	var result error
	for _, face := range set.faces {
		result = errors.Join(result, face.font.Close())
	}
	set.faces = nil
	return result
}

func (set *Set) choose(character rune, flags int) (*face, error) {
	primary := set.styles[flags&3]
	if primary == nil {
		return nil, fmt.Errorf("selected font lacks the recorded style %d", flags&3)
	}
	for _, candidate := range append([]*face{primary}, set.fallbacks...) {
		index, err := candidate.source.GlyphIndex(nil, character)
		if err != nil {
			return nil, err
		}
		if index != 0 {
			if candidate != primary {
				set.warnings[fmt.Sprintf("Explicit font fallback used for U+%04X", character)] = struct{}{}
			}
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("no selected font can render U+%04X; provide a compatible explicit fallback", character)
}

func (set *Set) glyph(dst draw.Image, x, baseline fixed.Int26_6, character rune, flags int, ink color.Color) (fixed.Int26_6, error) {
	selected, err := set.choose(character, flags)
	if err != nil {
		return 0, err
	}
	rectangle, mask, point, advance, ok := selected.font.Glyph(fixed.Point26_6{X: x, Y: baseline}, character)
	if !ok {
		return 0, fmt.Errorf("selected font cannot rasterize U+%04X", character)
	}
	if !rectangle.Empty() {
		draw.DrawMask(dst, rectangle, image.NewUniform(ink), image.Point{}, mask, point, draw.Over)
	}
	return advance, nil
}

// Cells reports the terminal cell width of one decoded rune.
func Cells(character rune) int {
	if unicode.Is(unicode.Mn, character) || unicode.Is(unicode.Me, character) {
		return 0
	}
	kind := width.LookupRune(character).Kind()
	if kind == width.EastAsianWide || kind == width.EastAsianFullwidth {
		return 2
	}
	return 1
}

// DrawCells paints actual terminal text at fixed cell positions.
func (set *Set) DrawCells(dst draw.Image, x, baseline int, text string, flags int, ink color.Color) error {
	for _, character := range text {
		if !unicode.IsSpace(character) {
			if _, err := set.glyph(dst, fixed.I(x), fixed.I(baseline), character, flags, ink); err != nil {
				return err
			}
		}
		x += Cells(character) * set.CellWidth
	}
	return nil
}

// Measure returns the selected fonts' actual advance for annotation text.
func (set *Set) Measure(text string) (float64, error) {
	var total fixed.Int26_6
	for _, character := range text {
		selected, err := set.choose(character, 0)
		if err != nil {
			return 0, err
		}
		advance, ok := selected.font.GlyphAdvance(character)
		if !ok {
			return 0, fmt.Errorf("selected font has no annotation advance for U+%04X", character)
		}
		total += advance
	}
	return float64(total) / 64, nil
}

// DrawText paints annotations without pretending they are product terminal cells.
func (set *Set) DrawText(dst draw.Image, x, baseline int, text string, ink color.Color) error {
	position := fixed.I(x)
	for _, character := range text {
		advance, err := set.glyph(dst, position, fixed.I(baseline), character, 0, ink)
		if err != nil {
			return err
		}
		position += advance
	}
	return nil
}

// Warnings returns explicit fallback use in deterministic order.
func (set *Set) Warnings() []string {
	result := make([]string, 0, len(set.warnings))
	for message := range set.warnings {
		result = append(result, message)
	}
	slices.Sort(result)
	return result
}

// Probe checks both printable ASCII metrics and actual Go glyph rasterization.
func Probe(filename string) (_ Info, err error) {
	set, err := Open(filename, 20, nil)
	if err != nil {
		return Info{}, err
	}
	defer func() { err = errors.Join(err, set.Close()) }()
	target := image.NewRGBA(image.Rect(0, 0, 10*set.CellWidth, 2*set.CellHeight))
	if err := set.DrawCells(target, 0, set.Ascent, "Glyph 123", 0, color.White); err != nil {
		return Info{}, err
	}
	return set.Info, nil
}
