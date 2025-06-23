package log

import (
	"fmt"
	"log/syslog"
	"regexp"
	"strings"
	"time"
)

// UseMock sets a mock logger as the default logger, and returns it.
func UseMock() *Mock {
	m := NewMock()
	_ = Set(m)
	return m
}

// NewMock creates a mock logger.
func NewMock() *Mock {
	return &Mock{impl{newMockWriter()}}
}

// NewWaitingMock creates a mock logger implementing the writer interface.
// It stores all logged messages in a buffer for inspection by test
// functions.
func NewWaitingMock() *WaitingMock {
	return &WaitingMock{impl{newWaitingMockWriter()}}
}

// Mock is a logger that stores all log messages in memory to be examined by a
// test.
type Mock struct {
	impl
}

// WaitingMock is a logger that stores all messages in memory to be examined by a test with methods
type WaitingMock struct {
	impl
}

// Mock implements the writer interface. It
// stores all logged messages in a buffer for inspection by test
// functions (via GetAll()) instead of sending them to syslog.
type mockWriter struct {
	logged    []string
	msgChan   chan<- string
	getChan   <-chan []string
	clearChan chan<- struct{}
	closeChan chan<- struct{}
}

var levelName = map[syslog.Priority]string{
	syslog.LOG_ERR:     "ERR",
	syslog.LOG_WARNING: "WARNING",
	syslog.LOG_INFO:    "INFO",
	syslog.LOG_DEBUG:   "DEBUG",
}

func (w *mockWriter) logAtLevel(p syslog.Priority, msg string, a ...interface{}) {
	w.msgChan <- fmt.Sprintf("%s: %s", levelName[p&7], fmt.Sprintf(msg, a...))
}

// newMockWriter returns a new mockWriter
func newMockWriter() *mockWriter {
	msgChan := make(chan string)
	getChan := make(chan []string)
	clearChan := make(chan struct{})
	closeChan := make(chan struct{})
	w := &mockWriter{
		logged:    []string{},
		msgChan:   msgChan,
		getChan:   getChan,
		clearChan: clearChan,
		closeChan: closeChan,
	}
	go func() {
		for {
			select {
			case logMsg := <-msgChan:
				w.logged = append(w.logged, logMsg)
			case getChan <- w.logged:
			case <-clearChan:
				w.logged = []string{}
			case <-closeChan:
				close(getChan)
				return
			}
		}
	}()
	return w
}

// GetAll returns all messages logged since instantiation or the last call to
// Clear().
//
// The caller must not modify the returned slice or its elements.
func (m *Mock) GetAll() []string {
	w := m.w.(*mockWriter)
	return <-w.getChan
}

// GetAllMatching returns all messages logged since instantiation or the last
// Clear() whose text matches the given regexp. The regexp is
// accepted as a string and compiled on the fly, because convenience
// is more important than performance.
//
// The caller must not modify the elements of the returned slice.
func (m *Mock) GetAllMatching(reString string) []string {
	var matches []string
	w := m.w.(*mockWriter)
	re := regexp.MustCompile(reString)
	for _, logMsg := range <-w.getChan {
		if re.MatchString(logMsg) {
			matches = append(matches, logMsg)
		}
	}
	return matches
}

func (m *Mock) ExpectMatch(reString string) error {
	results := m.GetAllMatching(reString)
	if len(results) == 0 {
		return fmt.Errorf("expected log line %q, got %q", reString, strings.Join(m.GetAll(), "\n"))
	}
	return nil
}

// Clear resets the log buffer.
func (m *Mock) Clear() {
	w := m.w.(*mockWriter)
	w.clearChan <- struct{}{}
}

type waitingMockWriter struct {
	logChan chan string
}

// newWaitingMockWriter returns a new waitingMockWriter
func newWaitingMockWriter() *waitingMockWriter {
	logChan := make(chan string, 1000)
	return &waitingMockWriter{
		logChan,
	}
}

func (m *waitingMockWriter) logAtLevel(p syslog.Priority, msg string, a ...interface{}) {
	m.logChan <- fmt.Sprintf("%s: %s", levelName[p&7], fmt.Sprintf(msg, a...))
}

// WaitForMatch returns the first log line matching a regex. It accepts a
// regexp string and timeout. If the timeout value is met before the
// matching pattern is read from the channel, an error is returned.
func (m *WaitingMock) WaitForMatch(reString string, timeout time.Duration) (string, error) {
	w := m.w.(*waitingMockWriter)
	deadline := time.After(timeout)
	re := regexp.MustCompile(reString)
	for {
		select {
		case logLine := <-w.logChan:
			if re.MatchString(logLine) {
				close(w.logChan)
				return logLine, nil
			}
		case <-deadline:
			return "", fmt.Errorf("timeout waiting for match: %q", reString)
		}
	}
}
