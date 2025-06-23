package log

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"log/syslog"
	"os"
	"strings"
	"sync"

	"github.com/jmhodges/clock"
	"golang.org/x/term"

	"github.com/letsencrypt/boulder/core"
)

// A Logger logs messages with explicit priority levels. It is
// implemented by a logging back-end as provided by New() or
// NewMock(). Any additions to this interface with format strings should be
// added to the govet configuration in .golangci.yml
type Logger interface {
	Err(msg string)
	Errf(format string, a ...interface{})
	Warning(msg string)
	Warningf(format string, a ...interface{})
	Info(msg string)
	Infof(format string, a ...interface{})
	InfoObject(string, interface{})
	Debug(msg string)
	Debugf(format string, a ...interface{})
	AuditInfo(msg string)
	AuditInfof(format string, a ...interface{})
	AuditObject(string, interface{})
	AuditErr(string)
	AuditErrf(format string, a ...interface{})
}

// impl implements Logger.
type impl struct {
	w writer
}

// singleton defines the object of a Singleton pattern
type singleton struct {
	once sync.Once
	log  Logger
}

// _Singleton is the single impl entity in memory
var _Singleton singleton

// The constant used to identify audit-specific messages
const auditTag = "[AUDIT]"

// New returns a new Logger that uses the given syslog.Writer as a backend
// and also writes to stdout/stderr. It is safe for concurrent use.
func New(log *syslog.Writer, stdoutLogLevel int, syslogLogLevel int) (Logger, error) {
	if log == nil {
		return nil, errors.New("Attempted to use a nil System Logger")
	}
	return &impl{
		&bothWriter{
			sync.Mutex{},
			log,
			newStdoutWriter(stdoutLogLevel),
			syslogLogLevel,
		},
	}, nil
}

// StdoutLogger returns a Logger that writes solely to stdout and stderr.
// It is safe for concurrent use.
func StdoutLogger(level int) Logger {
	return &impl{newStdoutWriter(level)}
}

func newStdoutWriter(level int) *stdoutWriter {
	prefix, clkFormat := getPrefix()
	return &stdoutWriter{
		prefix:    prefix,
		level:     level,
		clkFormat: clkFormat,
		clk:       clock.New(),
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		isatty:    term.IsTerminal(int(os.Stdout.Fd())),
	}
}

// initialize is used in unit tests and called by `Get` before the logger
// is fully set up.
func initialize() {
	const defaultPriority = syslog.LOG_INFO | syslog.LOG_LOCAL0
	syslogger, err := syslog.Dial("", "", defaultPriority, "test")
	if err != nil {
		panic(err)
	}
	logger, err := New(syslogger, int(syslog.LOG_DEBUG), int(syslog.LOG_DEBUG))
	if err != nil {
		panic(err)
	}

	_ = Set(logger)
}

// Set configures the singleton Logger. This method
// must only be called once, and before calling Get the
// first time.
func Set(logger Logger) (err error) {
	if _Singleton.log != nil {
		err = errors.New("You may not call Set after it has already been implicitly or explicitly set")
		_Singleton.log.Warning(err.Error())
	} else {
		_Singleton.log = logger
	}
	return
}

// Get obtains the singleton Logger. If Set has not been called first, this
// method initializes with basic defaults.  The basic defaults cannot error, and
// subsequent access to an already-set Logger also cannot error, so this method is
// error-safe.
func Get() Logger {
	_Singleton.once.Do(func() {
		if _Singleton.log == nil {
			initialize()
		}
	})

	return _Singleton.log
}

type writer interface {
	logAtLevel(syslog.Priority, string, ...interface{})
}

// bothWriter implements writer and writes to both syslog and stdout.
type bothWriter struct {
	sync.Mutex
	*syslog.Writer
	*stdoutWriter
	syslogLevel int
}

// stdoutWriter implements writer and writes just to stdout.
type stdoutWriter struct {
	// prefix is a set of information that is the same for every log line,
	// imitating what syslog emits for us when we use the syslog writer.
	prefix    string
	level     int
	clkFormat string
	clk       clock.Clock
	stdout    io.Writer
	stderr    io.Writer
	isatty    bool
}

func LogLineChecksum(line string) string {
	crc := crc32.ChecksumIEEE([]byte(line))
	// Using the hash.Hash32 doesn't make this any easier
	// as it also returns a uint32 rather than []byte
	buf := make([]byte, binary.MaxVarintLen32)
	binary.PutUvarint(buf, uint64(crc))
	return base64.RawURLEncoding.EncodeToString(buf)
}

func checkSummed(msg string) string {
	return fmt.Sprintf("%s %s", LogLineChecksum(msg), msg)
}

// logAtLevel logs the provided message at the appropriate level, writing to
// both stdout and the Logger
func (w *bothWriter) logAtLevel(level syslog.Priority, msg string, a ...interface{}) {
	var err error

	// Apply conditional formatting for f functions
	if a != nil {
		msg = fmt.Sprintf(msg, a...)
	}

	// Since messages are delimited by newlines, we have to escape any internal or
	// trailing newlines before generating the checksum or outputting the message.
	msg = strings.Replace(msg, "\n", "\\n", -1)

	w.Lock()
	defer w.Unlock()

	switch syslogAllowed := int(level) <= w.syslogLevel; level {
	case syslog.LOG_ERR:
		if syslogAllowed {
			err = w.Err(checkSummed(msg))
		}
	case syslog.LOG_WARNING:
		if syslogAllowed {
			err = w.Warning(checkSummed(msg))
		}
	case syslog.LOG_INFO:
		if syslogAllowed {
			err = w.Info(checkSummed(msg))
		}
	case syslog.LOG_DEBUG:
		if syslogAllowed {
			err = w.Debug(checkSummed(msg))
		}
	default:
		err = w.Err(fmt.Sprintf("%s (unknown logging level: %d)", checkSummed(msg), int(level)))
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write to syslog: %d %s (%s)\n", int(level), checkSummed(msg), err)
	}

	w.stdoutWriter.logAtLevel(level, msg)
}

// logAtLevel logs the provided message to stdout, or stderr if it is at Warning or Error level.
func (w *stdoutWriter) logAtLevel(level syslog.Priority, msg string, a ...interface{}) {
	if int(level) <= w.level {
		output := w.stdout
		if int(level) <= int(syslog.LOG_WARNING) {
			output = w.stderr
		}

		// Apply conditional formatting for f functions
		if a != nil {
			msg = fmt.Sprintf(msg, a...)
		}

		msg = strings.Replace(msg, "\n", "\\n", -1)

		var color string
		var reset string

		const red = "\033[31m\033[1m"
		const yellow = "\033[33m"
		const gray = "\033[37m\033[2m"

		if w.isatty {
			if int(level) == int(syslog.LOG_DEBUG) {
				color = gray
				reset = "\033[0m"
			} else if int(level) == int(syslog.LOG_WARNING) {
				color = yellow
				reset = "\033[0m"
			} else if int(level) <= int(syslog.LOG_ERR) {
				color = red
				reset = "\033[0m"
			}
		}

		if _, err := fmt.Fprintf(output, "%s%s %s%d %s %s%s\n",
			color,
			w.clk.Now().UTC().Format(w.clkFormat),
			w.prefix,
			int(level),
			core.Command(),
			checkSummed(msg),
			reset); err != nil {
			panic(fmt.Sprintf("failed to write to stdout: %v\n", err))
		}
	}
}

func (log *impl) auditAtLevel(level syslog.Priority, msg string, a ...interface{}) {
	msg = fmt.Sprintf("%s %s", auditTag, msg)
	log.w.logAtLevel(level, msg, a...)
}

// Err level messages are always marked with the audit tag, for special handling
// at the upstream system logger.
func (log *impl) Err(msg string) {
	log.Errf(msg)
}

// Errf level messages are always marked with the audit tag, for special handling
// at the upstream system logger.
func (log *impl) Errf(format string, a ...interface{}) {
	log.auditAtLevel(syslog.LOG_ERR, format, a...)
}

// Warning level messages pass through normally.
func (log *impl) Warning(msg string) {
	log.Warningf(msg)
}

// Warningf level messages pass through normally.
func (log *impl) Warningf(format string, a ...interface{}) {
	log.w.logAtLevel(syslog.LOG_WARNING, format, a...)
}

// Info level messages pass through normally.
func (log *impl) Info(msg string) {
	log.Infof(msg)
}

// Infof level messages pass through normally.
func (log *impl) Infof(format string, a ...interface{}) {
	log.w.logAtLevel(syslog.LOG_INFO, format, a...)
}

// InfoObject logs an INFO level JSON-serialized object message.
func (log *impl) InfoObject(msg string, obj interface{}) {
	jsonObj, err := json.Marshal(obj)
	if err != nil {
		log.auditAtLevel(syslog.LOG_ERR, fmt.Sprintf("Object for msg %q could not be serialized to JSON. Raw: %+v", msg, obj))
		return
	}

	log.Infof("%s JSON=%s", msg, jsonObj)
}

// Debug level messages pass through normally.
func (log *impl) Debug(msg string) {
	log.Debugf(msg)

}

// Debugf level messages pass through normally.
func (log *impl) Debugf(format string, a ...interface{}) {
	log.w.logAtLevel(syslog.LOG_DEBUG, format, a...)
}

// AuditInfo sends an INFO-severity message that is prefixed with the
// audit tag, for special handling at the upstream system logger.
func (log *impl) AuditInfo(msg string) {
	log.AuditInfof(msg)
}

// AuditInfof sends an INFO-severity message that is prefixed with the
// audit tag, for special handling at the upstream system logger.
func (log *impl) AuditInfof(format string, a ...interface{}) {
	log.auditAtLevel(syslog.LOG_INFO, format, a...)
}

// AuditObject sends an INFO-severity JSON-serialized object message that is prefixed
// with the audit tag, for special handling at the upstream system logger.
func (log *impl) AuditObject(msg string, obj interface{}) {
	jsonObj, err := json.Marshal(obj)
	if err != nil {
		log.auditAtLevel(syslog.LOG_ERR, fmt.Sprintf("Object for msg %q could not be serialized to JSON. Raw: %+v", msg, obj))
		return
	}

	log.auditAtLevel(syslog.LOG_INFO, fmt.Sprintf("%s JSON=%s", msg, jsonObj))
}

// AuditErr can format an error for auditing; it does so at ERR level.
func (log *impl) AuditErr(msg string) {
	log.AuditErrf(msg)
}

// AuditErrf can format an error for auditing; it does so at ERR level.
func (log *impl) AuditErrf(format string, a ...interface{}) {
	log.auditAtLevel(syslog.LOG_ERR, format, a...)
}
