package validator

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/log"
)

var errInvalidChecksum = errors.New("invalid checksum length")

type Validator struct {
	// mu guards patterns and tailers to prevent Shutdown racing monitor
	mu sync.Mutex

	// patterns is the list of glob patterns to monitor with filepath.Glob for logs
	patterns []string

	// tailers is a map of filenames to the tailer which are currently being tailed
	tailers map[string]*tail.Tail

	// monitorCancel cancels the monitor's context, so it exits
	monitorCancel context.CancelFunc

	lineCounter *prometheus.CounterVec
	log         log.Logger
}

// New Validator monitoring paths, which is a list of file globs.
func New(patterns []string, logger log.Logger, stats prometheus.Registerer) *Validator {
	lineCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "log_lines",
		Help: "A counter of log lines processed, with status",
	}, []string{"filename", "status"})
	stats.MustRegister(lineCounter)

	monitorContext, monitorCancel := context.WithCancel(context.Background())

	v := &Validator{
		patterns:      patterns,
		tailers:       map[string]*tail.Tail{},
		log:           logger,
		monitorCancel: monitorCancel,
		lineCounter:   lineCounter,
	}

	go v.monitor(monitorContext)

	return v
}

// pollPaths expands v.patterns and calls v.tailValidateFile on each resulting file
func (v *Validator) pollPaths() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, pattern := range v.patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			v.log.Err(err.Error())
		}

		for _, path := range paths {
			if _, ok := v.tailers[path]; ok {
				// We are already tailing this file
				continue
			}

			t, err := tail.TailFile(path, tail.Config{
				ReOpen:        true,
				MustExist:     false, // sometimes files won't exist, so we must tolerate that
				Follow:        true,
				Logger:        tailLogger{v.log},
				CompleteLines: true,
			})
			if err != nil {
				// TailFile shouldn't error when MustExist is false
				v.log.Errf("unexpected error from TailFile: %v", err)
			}

			go v.tailValidate(path, t.Lines)

			v.tailers[path] = t
		}
	}
}

// Monitor calls v.pollPaths every minute until its context is cancelled
func (v *Validator) monitor(ctx context.Context) {
	for {
		v.pollPaths()

		// Wait a minute, unless cancelled
		timer := time.NewTimer(time.Minute)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (v *Validator) tailValidate(filename string, lines chan *tail.Line) {
	// Emit no more than 1 error line per second. This prevents consuming large
	// amounts of disk space in case there is problem that causes all log lines to
	// be invalid.
	outputLimiter := time.NewTicker(time.Second)
	defer outputLimiter.Stop()

	for line := range lines {
		if line.Err != nil {
			v.log.Errf("error while tailing %s: %s", filename, line.Err)
			continue
		}
		err := lineValid(line.Text)
		if err != nil {
			if errors.Is(err, errInvalidChecksum) {
				v.lineCounter.WithLabelValues(filename, "invalid checksum length").Inc()
			} else {
				v.lineCounter.WithLabelValues(filename, "bad").Inc()
			}
			select {
			case <-outputLimiter.C:
				v.log.Errf("%s: %s %q", filename, err, line.Text)
			default:
			}
		} else {
			v.lineCounter.WithLabelValues(filename, "ok").Inc()
		}
	}
}

// Shutdown should be called before process shutdown
func (v *Validator) Shutdown() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.monitorCancel()

	for _, t := range v.tailers {
		// The tail module seems to have a race condition that will generate
		// errors like this on shutdown:
		// failed to stop tailing file: <filename>: Failed to detect creation of
		// <filename>: inotify watcher has been closed
		// This is probably related to the module's shutdown logic triggering the
		// "reopen" code path for files that are removed and then recreated.
		// These errors are harmless so we ignore them to allow clean shutdown.
		_ = t.Stop()
		t.Cleanup()
	}
}

func lineValid(text string) error {
	// Line format should match the following rsyslog omfile template:
	//
	//   template( name="LELogFormat" type="list" ) {
	//  	property(name="timereported" dateFormat="rfc3339")
	//  	constant(value=" ")
	//  	property(name="hostname" field.delimiter="46" field.number="1")
	//  	constant(value=" datacenter ")
	//  	property(name="syslogseverity")
	//  	constant(value=" ")
	//  	property(name="syslogtag")
	//  	property(name="msg" spifno1stsp="on" )
	//  	property(name="msg" droplastlf="on" )
	//  	constant(value="\n")
	//   }
	//
	// This should result in a log line that looks like this:
	//   timestamp hostname datacenter syslogseverity binary-name[pid]: checksum msg

	fields := strings.Split(text, " ")
	const errorPrefix = "log-validator:"
	// Extract checksum from line
	if len(fields) < 6 {
		return fmt.Errorf("%s line doesn't match expected format", errorPrefix)
	}
	checksum := fields[5]
	_, err := base64.RawURLEncoding.DecodeString(checksum)
	if err != nil || len(checksum) != 7 {
		return fmt.Errorf(
			"%s expected a 7 character base64 raw URL decodable string, got %q: %w",
			errorPrefix,
			checksum,
			errInvalidChecksum,
		)
	}

	// Reconstruct just the message portion of the line
	line := strings.Join(fields[6:], " ")

	// If we are fed our own output, treat it as always valid. This
	// prevents runaway scenarios where we generate ever-longer output.
	if strings.Contains(text, errorPrefix) {
		return nil
	}
	// Check the extracted checksum against the computed checksum
	if computedChecksum := log.LogLineChecksum(line); checksum != computedChecksum {
		return fmt.Errorf("%s invalid checksum (expected %q, got %q)", errorPrefix, computedChecksum, checksum)
	}
	return nil
}

// ValidateFile validates a single file and returns
func ValidateFile(filename string) error {
	file, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	badFile := false
	for i, line := range strings.Split(string(file), "\n") {
		if line == "" {
			continue
		}
		err := lineValid(line)
		if err != nil {
			badFile = true
			fmt.Fprintf(os.Stderr, "[line %d] %s: %s\n", i+1, err, line)
		}
	}

	if badFile {
		return errors.New("file contained invalid lines")
	}
	return nil
}
