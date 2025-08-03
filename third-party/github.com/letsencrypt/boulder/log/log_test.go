package log

import (
	"bytes"
	"fmt"
	"log/syslog"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/test"
)

const stdoutLevel = 7
const syslogLevel = 7

func setup(t *testing.T) *impl {
	// Write all logs to UDP on a high port so as to not bother the system
	// which is running the test
	writer, err := syslog.Dial("udp", "127.0.0.1:65530", syslog.LOG_INFO|syslog.LOG_LOCAL0, "")
	test.AssertNotError(t, err, "Could not construct syslog object")

	logger, err := New(writer, stdoutLevel, syslogLevel)
	test.AssertNotError(t, err, "Could not construct syslog object")
	impl, ok := logger.(*impl)
	if !ok {
		t.Fatalf("Wrong type returned from New: %T", logger)
	}
	return impl
}

func TestConstruction(t *testing.T) {
	t.Parallel()
	_ = setup(t)
}

func TestSingleton(t *testing.T) {
	t.Parallel()
	log1 := Get()
	test.AssertNotNil(t, log1, "Logger shouldn't be nil")

	log2 := Get()
	test.AssertEquals(t, log1, log2)

	audit := setup(t)

	// Should not work
	err := Set(audit)
	test.AssertError(t, err, "Can't re-set")

	// Verify no change
	log4 := Get()

	// Verify that log4 != log3
	test.AssertNotEquals(t, log4, audit)

	// Verify that log4 == log2 == log1
	test.AssertEquals(t, log4, log2)
	test.AssertEquals(t, log4, log1)
}

func TestConstructionNil(t *testing.T) {
	t.Parallel()
	_, err := New(nil, stdoutLevel, syslogLevel)
	test.AssertError(t, err, "Nil shouldn't be permitted.")
}

func TestEmit(t *testing.T) {
	t.Parallel()
	log := setup(t)

	log.AuditInfo("test message")
}

func TestEmitEmpty(t *testing.T) {
	t.Parallel()
	log := setup(t)

	log.AuditInfo("")
}

func TestStdoutLogger(t *testing.T) {
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	logger := &impl{
		&stdoutWriter{
			prefix:    "prefix ",
			level:     7,
			clkFormat: "2006-01-02",
			clk:       clock.NewFake(),
			stdout:    stdout,
			stderr:    stderr,
		},
	}

	logger.AuditErr("Error Audit")
	logger.Warning("Warning log")
	logger.Info("Info log")

	test.AssertEquals(t, stdout.String(), "1970-01-01 prefix 6 log.test pcbo7wk Info log\n")
	test.AssertEquals(t, stderr.String(), "1970-01-01 prefix 3 log.test 46_ghQg [AUDIT] Error Audit\n1970-01-01 prefix 4 log.test 97r2xAw Warning log\n")
}

func TestSyslogMethods(t *testing.T) {
	t.Parallel()
	impl := setup(t)

	impl.AuditInfo("audit-logger_test.go: audit-info")
	impl.AuditErr("audit-logger_test.go: audit-err")
	impl.Debug("audit-logger_test.go: debug")
	impl.Err("audit-logger_test.go: err")
	impl.Info("audit-logger_test.go: info")
	impl.Warning("audit-logger_test.go: warning")
	impl.AuditInfof("audit-logger_test.go: %s", "audit-info")
	impl.AuditErrf("audit-logger_test.go: %s", "audit-err")
	impl.Debugf("audit-logger_test.go: %s", "debug")
	impl.Errf("audit-logger_test.go: %s", "err")
	impl.Infof("audit-logger_test.go: %s", "info")
	impl.Warningf("audit-logger_test.go: %s", "warning")
}

func TestAuditObject(t *testing.T) {
	t.Parallel()

	log := NewMock()

	// Test a simple object
	log.AuditObject("Prefix", "String")
	if len(log.GetAllMatching("[AUDIT]")) != 1 {
		t.Errorf("Failed to audit log simple object")
	}

	// Test a system object
	log.Clear()
	log.AuditObject("Prefix", t)
	if len(log.GetAllMatching("[AUDIT]")) != 1 {
		t.Errorf("Failed to audit log system object")
	}

	// Test a complex object
	log.Clear()
	type validObj struct {
		A string
		B string
	}
	var valid = validObj{A: "B", B: "C"}
	log.AuditObject("Prefix", valid)
	if len(log.GetAllMatching("[AUDIT]")) != 1 {
		t.Errorf("Failed to audit log complex object")
	}

	// Test logging an unserializable object
	log.Clear()
	type invalidObj struct {
		A chan string
	}

	var invalid = invalidObj{A: make(chan string)}
	log.AuditObject("Prefix", invalid)
	if len(log.GetAllMatching("[AUDIT]")) != 1 {
		t.Errorf("Failed to audit log unserializable object %v", log.GetAllMatching("[AUDIT]"))
	}
}

func TestTransmission(t *testing.T) {
	t.Parallel()

	l, err := newUDPListener("127.0.0.1:0")
	test.AssertNotError(t, err, "Failed to open log server")
	defer func() {
		err = l.Close()
		test.AssertNotError(t, err, "listener.Close returned error")
	}()

	fmt.Printf("Going to %s\n", l.LocalAddr().String())
	writer, err := syslog.Dial("udp", l.LocalAddr().String(), syslog.LOG_INFO|syslog.LOG_LOCAL0, "")
	test.AssertNotError(t, err, "Failed to find connect to log server")

	impl, err := New(writer, stdoutLevel, syslogLevel)
	test.AssertNotError(t, err, "Failed to construct audit logger")

	data := make([]byte, 128)

	impl.AuditInfo("audit-logger_test.go: audit-info")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.AuditErr("audit-logger_test.go: audit-err")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Debug("audit-logger_test.go: debug")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Err("audit-logger_test.go: err")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Info("audit-logger_test.go: info")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Warning("audit-logger_test.go: warning")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.AuditInfof("audit-logger_test.go: %s", "audit-info")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.AuditErrf("audit-logger_test.go: %s", "audit-err")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Debugf("audit-logger_test.go: %s", "debug")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Errf("audit-logger_test.go: %s", "err")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Infof("audit-logger_test.go: %s", "info")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")

	impl.Warningf("audit-logger_test.go: %s", "warning")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")
}

func TestSyslogLevels(t *testing.T) {
	t.Parallel()

	l, err := newUDPListener("127.0.0.1:0")
	test.AssertNotError(t, err, "Failed to open log server")
	defer func() {
		err = l.Close()
		test.AssertNotError(t, err, "listener.Close returned error")
	}()

	fmt.Printf("Going to %s\n", l.LocalAddr().String())
	writer, err := syslog.Dial("udp", l.LocalAddr().String(), syslog.LOG_INFO|syslog.LOG_LOCAL0, "")
	test.AssertNotError(t, err, "Failed to find connect to log server")

	// create a logger with syslog level debug
	impl, err := New(writer, stdoutLevel, int(syslog.LOG_DEBUG))
	test.AssertNotError(t, err, "Failed to construct audit logger")

	data := make([]byte, 512)

	// debug messages should be sent to the logger
	impl.Debug("log_test.go: debug")
	_, _, err = l.ReadFrom(data)
	test.AssertNotError(t, err, "Failed to find packet")
	test.Assert(t, strings.Contains(string(data), "log_test.go: debug"), "Failed to find log message")

	// create a logger with syslog level info
	impl, err = New(writer, stdoutLevel, int(syslog.LOG_INFO))
	test.AssertNotError(t, err, "Failed to construct audit logger")

	// debug messages should not be sent to the logger
	impl.Debug("log_test.go: debug")
	n, _, err := l.ReadFrom(data)
	if n != 0 && err == nil {
		t.Error("Failed to withhold debug log message")
	}
}

func newUDPListener(addr string) (*net.UDPConn, error) {
	l, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, err
	}
	err = l.SetDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		return nil, err
	}
	err = l.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		return nil, err
	}
	err = l.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	if err != nil {
		return nil, err
	}
	return l.(*net.UDPConn), nil
}

// TestStdoutFailure tests that audit logging with a bothWriter panics if stdout
// becomes unavailable.
func TestStdoutFailure(t *testing.T) {
	// Save the stdout fd so we can restore it later
	saved := os.Stdout

	// Create a throw-away pipe FD to replace stdout with
	_, w, err := os.Pipe()
	test.AssertNotError(t, err, "failed to create pipe")
	os.Stdout = w

	// Setup the logger
	log := setup(t)

	// Close Stdout so that the fmt.Printf in bothWriter's logAtLevel
	// function will return an err on next log.
	err = os.Stdout.Close()
	test.AssertNotError(t, err, "failed to close stdout")

	// Defer a function that will check if there was a panic to recover from. If
	// there wasn't then the test should fail, we were able to AuditInfo when
	// Stdout was inoperable.
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Errorf("log.AuditInfo with Stdout closed did not panic")
		}

		// Restore stdout so that subsequent tests don't fail
		os.Stdout = saved
	}()

	// Try to audit log something
	log.AuditInfo("This should cause a panic, stdout is closed!")
}

func TestLogAtLevelEscapesNewlines(t *testing.T) {
	var buf bytes.Buffer
	w := &bothWriter{sync.Mutex{},
		nil,
		&stdoutWriter{
			stdout: &buf,
			clk:    clock.NewFake(),
			level:  6,
		},
		-1,
	}
	w.logAtLevel(6, "foo\nbar")

	test.Assert(t, strings.Contains(buf.String(), "foo\\nbar"), "failed to escape newline")
}
