package shared

import (
	"fmt"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"

	"github.com/cli/cli/v2/pkg/iostreams"
)

// UploadProgressPrinter renders per-asset progress, updating each line in-place
// when stderr supports TTY control codes. Otherwise it falls back to textual logs.
type UploadProgressPrinter struct {
	io      *iostreams.IOStreams
	total   int
	dynamic bool
	textual bool

	mu            sync.Mutex
	lines         []string
	lineStates    []*uploadLine
	lastPercent   map[int]int
	lastUploaded  map[int]int64
	renderedLines int

	spinnerTicker *time.Ticker
	spinnerStop   chan struct{}
	spinnerActive bool
}

type uploadLine struct {
	asset        AssetForUpload
	uploaded     int64
	status       lineStatus
	err          error
	spinnerFrame int
}

type lineStatus int

const (
	lineStatusUploading lineStatus = iota
	lineStatusSuccess
	lineStatusFailure
)

var spinnerFrames = spinner.CharSets[11]

const spinnerInterval = 120 * time.Millisecond

func NewUploadProgressPrinter(io *iostreams.IOStreams, assets []*AssetForUpload) *UploadProgressPrinter {
	if len(assets) == 0 {
		return nil
	}

	AssignAssetOrdinals(assets)

	p := &UploadProgressPrinter{
		io:           io,
		total:        len(assets),
		dynamic:      io.ProgressIndicatorEnabled() && io.IsStderrTTY() && !io.GetSpinnerDisabled(),
		textual:      io.GetSpinnerDisabled(),
		lineStates:   make([]*uploadLine, len(assets)),
		lines:        make([]string, 0, len(assets)),
		lastPercent:  make(map[int]int),
		lastUploaded: make(map[int]int64),
	}

	return p
}

func (p *UploadProgressPrinter) Callbacks() *UploadCallbacks {
	if p == nil {
		return nil
	}

	callbacks := &UploadCallbacks{
		OnUploadStart:    p.onStart,
		OnUploadComplete: p.onComplete,
	}

	if p.dynamic {
		callbacks.OnUploadProgress = p.onProgress
	}

	return callbacks
}

func (p *UploadProgressPrinter) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopSpinnerLocked()
	p.renderLocked()
}

func (p *UploadProgressPrinter) onStart(asset AssetForUpload) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ordinal := p.ordinal(asset)
	p.lastPercent[ordinal] = -1
	p.lastUploaded[ordinal] = 0

	if !p.dynamic {
		if p.textual {
			p.printLine("[%d/%d] Uploading %s%s", ordinal, p.total, asset.Name, p.sizeSuffix(asset))
		}
		return
	}

	idx := ordinal - 1
	if idx < 0 || idx >= len(p.lineStates) {
		return
	}

	p.lineStates[idx] = &uploadLine{
		asset:    asset,
		status:   lineStatusUploading,
		uploaded: 0,
	}
	p.ensureSpinnerLoopLocked()
	p.updateLinesLocked()
	p.renderLocked()
}

func (p *UploadProgressPrinter) onProgress(asset AssetForUpload, uploaded int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.dynamic {
		return
	}

	ordinal := p.ordinal(asset)
	idx := ordinal - 1
	if idx < 0 || idx >= len(p.lineStates) {
		return
	}

	line := p.lineStates[idx]
	if line == nil {
		return
	}

	if uploaded < p.lastUploaded[ordinal] {
		p.lastPercent[ordinal] = -1
	}

	p.lastUploaded[ordinal] = uploaded

	percent := p.percent(asset, uploaded)
	if percent == p.lastPercent[ordinal] && !(asset.Size > 0 && uploaded >= asset.Size) {
		return
	}

	p.lastPercent[ordinal] = percent
	line = p.lineStates[idx]
	if line == nil {
		return
	}
	line.uploaded = uploaded
	p.updateLinesLocked()
	p.renderLocked()
}

func (p *UploadProgressPrinter) onComplete(asset AssetForUpload, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ordinal := p.ordinal(asset)

	if !p.dynamic {
		if err != nil {
			p.printLine("[%d/%d] Failed uploading %s: %v", ordinal, p.total, asset.Name, err)
		} else if p.textual {
			p.printLine("[%d/%d] Uploaded %s", ordinal, p.total, asset.Name)
		}
		return
	}

	idx := ordinal - 1
	if idx < 0 || idx >= len(p.lineStates) {
		return
	}

	line := p.lineStates[idx]
	if line == nil {
		return
	}

	if err != nil {
		line.status = lineStatusFailure
		line.err = err
	} else {
		line.status = lineStatusSuccess
	}

	if line.status != lineStatusUploading && p.spinnerActive && !p.hasUploadingLocked() {
		p.stopSpinnerLocked()
	}

	p.updateLinesLocked()
	p.renderLocked()
}

func (p *UploadProgressPrinter) render() {
	if !p.dynamic || len(p.lines) == 0 {
		return
	}

	if p.renderedLines == 0 {
		for _, line := range p.lines {
			fmt.Fprintf(p.io.ErrOut, "%s\n", line)
		}
		p.renderedLines = len(p.lines)
		return
	}

	if p.renderedLines > 0 {
		fmt.Fprintf(p.io.ErrOut, "\x1b[%dA", p.renderedLines)
	}

	for _, line := range p.lines {
		fmt.Fprintf(p.io.ErrOut, "\x1b[2K\r%s\n", line)
	}

	p.renderedLines = len(p.lines)
}

func (p *UploadProgressPrinter) renderLocked() {
	if !p.dynamic || len(p.lines) == 0 {
		return
	}
	p.render()
}

func (p *UploadProgressPrinter) progressLabel(asset AssetForUpload, uploaded int64) string {
	ordinal := p.ordinal(asset)
	prefix := fmt.Sprintf("[%d/%d]", ordinal, p.total)

	if asset.Size > 0 {
		percent := p.percent(asset, uploaded)
		if uploaded > asset.Size {
			uploaded = asset.Size
		}
		return fmt.Sprintf("%s Uploading %s %3d%% (%s/%s)",
			prefix,
			asset.Name,
			percent,
			humanize.IBytes(uint64(max(uploaded, 0))),
			humanize.IBytes(uint64(asset.Size)))
	}

	return fmt.Sprintf("%s Uploading %s (%s)", prefix, asset.Name, humanize.IBytes(uint64(max(uploaded, 0))))
}

func (p *UploadProgressPrinter) sizeSuffix(asset AssetForUpload) string {
	if asset.Size <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%s)", humanize.IBytes(uint64(asset.Size)))
}

func (p *UploadProgressPrinter) percent(asset AssetForUpload, uploaded int64) int {
	if asset.Size <= 0 {
		return -1
	}
	if uploaded < 0 {
		uploaded = 0
	}
	percent := int(uploaded * 100 / asset.Size)
	if percent > 100 {
		return 100
	}
	return percent
}

func (p *UploadProgressPrinter) printLine(format string, args ...any) {
	fmt.Fprintf(p.io.ErrOut, format+"\n", args...)
}

func (p *UploadProgressPrinter) ordinal(asset AssetForUpload) int {
	if asset.Ordinal > 0 {
		return asset.Ordinal
	}
	return 1
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (p *UploadProgressPrinter) updateLinesLocked() {
	if !p.dynamic {
		return
	}

	p.lines = p.lines[:0]
	cs := p.io.ColorScheme()

	for _, line := range p.lineStates {
		if line == nil {
			continue
		}

		var prefix string
		switch line.status {
		case lineStatusUploading:
			prefix = spinnerFrames[line.spinnerFrame%len(spinnerFrames)]
		case lineStatusSuccess:
			prefix = cs.SuccessIcon()
		case lineStatusFailure:
			prefix = cs.FailureIcon()
		}

		body := p.bodyForLine(line)
		p.lines = append(p.lines, fmt.Sprintf("%s %s", prefix, body))
	}
}

func (p *UploadProgressPrinter) bodyForLine(line *uploadLine) string {
	switch line.status {
	case lineStatusUploading:
		return p.progressLabel(line.asset, line.uploaded)
	case lineStatusSuccess:
		return fmt.Sprintf("[%d/%d] Uploaded %s", p.ordinal(line.asset), p.total, line.asset.Name)
	case lineStatusFailure:
		return fmt.Sprintf("[%d/%d] Failed uploading %s: %v", p.ordinal(line.asset), p.total, line.asset.Name, line.err)
	default:
		return ""
	}
}

func (p *UploadProgressPrinter) ensureSpinnerLoopLocked() {
	if !p.dynamic || p.spinnerActive {
		return
	}
	p.spinnerTicker = time.NewTicker(spinnerInterval)
	p.spinnerStop = make(chan struct{})
	p.spinnerActive = true

	go p.spin()
}

func (p *UploadProgressPrinter) spin() {
	for {
		select {
		case <-p.spinnerTicker.C:
			p.mu.Lock()
			updated := false
			for _, line := range p.lineStates {
				if line != nil && line.status == lineStatusUploading {
					line.spinnerFrame = (line.spinnerFrame + 1) % len(spinnerFrames)
					updated = true
				}
			}
			if updated {
				p.updateLinesLocked()
				p.renderLocked()
			}
			p.mu.Unlock()
		case <-p.spinnerStop:
			p.spinnerTicker.Stop()
			return
		}
	}
}

func (p *UploadProgressPrinter) stopSpinnerLocked() {
	if !p.spinnerActive {
		return
	}
	close(p.spinnerStop)
	p.spinnerActive = false
}

func (p *UploadProgressPrinter) hasUploadingLocked() bool {
	for _, line := range p.lineStates {
		if line != nil && line.status == lineStatusUploading {
			return true
		}
	}
	return false
}
