package download

import (
	"fmt"
	"io"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
)

type ProgressReader struct {
	reader    io.Reader
	total     int64
	current   int64
	startTime time.Time
	io        *iostreams.IOStreams
	name      string
	lastPrint time.Time
}

func NewProgressReader(reader io.Reader, total int64, name string, io *iostreams.IOStreams) *ProgressReader {
	return &ProgressReader{
		reader:    reader,
		total:     total,
		current:   0,
		startTime: time.Now(),
		io:        io,
		name:      name,
		lastPrint: time.Now(),
	}
}

func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	pr.current += int64(n)
	
	now := time.Now()
	if now.Sub(pr.lastPrint) >= 100*time.Millisecond || err == io.EOF {
		pr.updateProgress()
		pr.lastPrint = now
	}
	
	return n, err
}

func (pr *ProgressReader) updateProgress() {
	if pr.total <= 0 {
		return
	}
	
	elapsed := time.Since(pr.startTime)
	percentage := float64(pr.current) / float64(pr.total) * 100
	
	var speed float64
	if elapsed.Seconds() > 0 {
		speed = float64(pr.current) / elapsed.Seconds()
	}
	
	currentSize := formatBytes(pr.current)
	totalSize := formatBytes(pr.total)
	speedStr := formatBytes(int64(speed)) + "/s"
	
	barWidth := 20
	completed := int(float64(barWidth) * percentage / 100)
	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < completed {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	
	cs := pr.io.ColorScheme()
	progressLine := fmt.Sprintf("\r%s %s %s/%s (%.1f%%) [%s]",
		cs.Bold("Downloading "+pr.name+"..."),
		bar,
		currentSize,
		totalSize,
		percentage,
		cs.Cyan(speedStr),
	)
	
	fmt.Fprint(pr.io.ErrOut, progressLine)
	
	if pr.current >= pr.total {
		fmt.Fprint(pr.io.ErrOut, "\n")
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
