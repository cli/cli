package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	validPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": { "http-01": true }
}`)
	invalidPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": { "nonsense": true }
}`)
	noChallengesPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false
}`)

	emptyChallengesPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": {}
}`)
)

func TestPAConfigUnmarshal(t *testing.T) {
	var pc1 PAConfig
	err := json.Unmarshal(validPAConfig, &pc1)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertNotError(t, pc1.CheckChallenges(), "Flagged valid challenges as bad")

	var pc2 PAConfig
	err = json.Unmarshal(invalidPAConfig, &pc2)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc2.CheckChallenges(), "Considered invalid challenges as good")

	var pc3 PAConfig
	err = json.Unmarshal(noChallengesPAConfig, &pc3)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc3.CheckChallenges(), "Disallow empty challenges map")

	var pc4 PAConfig
	err = json.Unmarshal(emptyChallengesPAConfig, &pc4)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc4.CheckChallenges(), "Disallow empty challenges map")
}

func TestMysqlLogger(t *testing.T) {
	log := blog.UseMock()
	mLog := mysqlLogger{log}

	testCases := []struct {
		args     []interface{}
		expected string
	}{
		{
			[]interface{}{nil},
			`ERR: [AUDIT] [mysql] <nil>`,
		},
		{
			[]interface{}{""},
			`ERR: [AUDIT] [mysql] `,
		},
		{
			[]interface{}{"Sup ", 12345, " Sup sup"},
			`ERR: [AUDIT] [mysql] Sup 12345 Sup sup`,
		},
	}

	for _, tc := range testCases {
		// mysqlLogger proxies blog.AuditLogger to provide a Print() method
		mLog.Print(tc.args...)
		logged := log.GetAll()
		// Calling Print should produce the expected output
		test.AssertEquals(t, len(logged), 1)
		test.AssertEquals(t, logged[0], tc.expected)
		log.Clear()
	}
}

func TestCaptureStdlibLog(t *testing.T) {
	logger := blog.UseMock()
	oldDest := log.Writer()
	defer func() {
		log.SetOutput(oldDest)
	}()
	log.SetOutput(logWriter{logger})
	log.Print("thisisatest")
	results := logger.GetAllMatching("thisisatest")
	if len(results) != 1 {
		t.Fatalf("Expected logger to receive 'thisisatest', got: %s",
			strings.Join(logger.GetAllMatching(".*"), "\n"))
	}
}

func TestVersionString(t *testing.T) {
	core.BuildID = "TestBuildID"
	core.BuildTime = "RightNow!"
	core.BuildHost = "Localhost"

	versionStr := VersionString()
	expected := fmt.Sprintf("Versions: cmd.test=(TestBuildID RightNow!) Golang=(%s) BuildHost=(Localhost)", runtime.Version())
	test.AssertEquals(t, versionStr, expected)
}

func TestReadConfigFile(t *testing.T) {
	err := ReadConfigFile("", nil)
	test.AssertError(t, err, "ReadConfigFile('') did not error")

	type config struct {
		NotifyMailer struct {
			DB DBConfig
			SMTPConfig
		}
		Syslog SyslogConfig
	}
	var c config
	err = ReadConfigFile("../test/config/notify-mailer.json", &c)
	test.AssertNotError(t, err, "ReadConfigFile(../test/config/notify-mailer.json) errored")
	test.AssertEquals(t, c.NotifyMailer.SMTPConfig.Server, "localhost")
}

func TestLogWriter(t *testing.T) {
	mock := blog.UseMock()
	lw := logWriter{mock}
	_, _ = lw.Write([]byte("hi\n"))
	lines := mock.GetAllMatching(".*")
	test.AssertEquals(t, len(lines), 1)
	test.AssertEquals(t, lines[0], "INFO: hi")
}

func TestGRPCLoggerWarningFilter(t *testing.T) {
	m := blog.NewMock()
	l := grpcLogger{m}
	l.Warningln("asdf", "qwer")
	lines := m.GetAllMatching(".*")
	test.AssertEquals(t, len(lines), 1)

	m = blog.NewMock()
	l = grpcLogger{m}
	l.Warningln("Server.processUnaryRPC failed to write status: connection error: desc = \"transport is closing\"")
	lines = m.GetAllMatching(".*")
	test.AssertEquals(t, len(lines), 0)
}

func Test_newVersionCollector(t *testing.T) {
	// 'buildTime'
	core.BuildTime = core.Unspecified
	version := newVersionCollector()
	// Default 'Unspecified' should emit 'Unspecified'.
	test.AssertMetricWithLabelsEquals(t, version, prometheus.Labels{"buildTime": core.Unspecified}, 1)
	// Parsable UnixDate should emit UnixTime.
	now := time.Now().UTC()
	core.BuildTime = now.Format(time.UnixDate)
	version = newVersionCollector()
	test.AssertMetricWithLabelsEquals(t, version, prometheus.Labels{"buildTime": now.Format(time.RFC3339)}, 1)
	// Unparsable timestamp should emit 'Unsparsable'.
	core.BuildTime = "outta time"
	version = newVersionCollector()
	test.AssertMetricWithLabelsEquals(t, version, prometheus.Labels{"buildTime": "Unparsable"}, 1)

	// 'buildId'
	expectedBuildID := "TestBuildId"
	core.BuildID = expectedBuildID
	version = newVersionCollector()
	test.AssertMetricWithLabelsEquals(t, version, prometheus.Labels{"buildId": expectedBuildID}, 1)

	// 'goVersion'
	test.AssertMetricWithLabelsEquals(t, version, prometheus.Labels{"goVersion": runtime.Version()}, 1)
}

func loadConfigFile(t *testing.T, path string) *os.File {
	cf, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return cf
}

func TestFailedConfigValidation(t *testing.T) {
	type FooConfig struct {
		VitalValue       string `yaml:"vitalValue" validate:"required"`
		VoluntarilyVoid  string `yaml:"voluntarilyVoid"`
		VisciouslyVetted string `yaml:"visciouslyVetted" validate:"omitempty,endswith=baz"`
	}

	// Violates 'endswith' tag JSON.
	cf := loadConfigFile(t, "testdata/1_missing_endswith.json")
	defer cf.Close()
	err := ValidateJSONConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'endswith'")

	// Violates 'endswith' tag YAML.
	cf = loadConfigFile(t, "testdata/1_missing_endswith.yaml")
	defer cf.Close()
	err = ValidateYAMLConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'endswith'")

	// Violates 'required' tag JSON.
	cf = loadConfigFile(t, "testdata/2_missing_required.json")
	defer cf.Close()
	err = ValidateJSONConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'required'")

	// Violates 'required' tag YAML.
	cf = loadConfigFile(t, "testdata/2_missing_required.yaml")
	defer cf.Close()
	err = ValidateYAMLConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'required'")
}

func TestFailExit(t *testing.T) {
	// Test that when Fail is called with a `defer AuditPanic()`,
	// the program exits with a non-zero exit code and logs
	// the result (but not stack trace).
	// Inspired by https://go.dev/talks/2014/testing.slide#23
	if os.Getenv("TIME_TO_DIE") == "1" {
		defer AuditPanic()
		Fail("tears in the rain")
		return
	}

	// gosec points out that os.Args[0] is tainted, but we only run this as a test
	// so we are not worried about it containing an untrusted value.
	//nolint:gosec
	cmd := exec.Command(os.Args[0], "-test.run=TestFailExit")
	cmd.Env = append(os.Environ(), "TIME_TO_DIE=1")
	output, err := cmd.CombinedOutput()
	test.AssertError(t, err, "running a failing program")
	test.AssertContains(t, string(output), "[AUDIT] tears in the rain")
	// "goroutine" usually shows up in stack traces, so we check it
	// to make sure we didn't print a stack trace.
	test.AssertNotContains(t, string(output), "goroutine")
}

func testPanicStackTraceHelper() {
	var x *int
	*x = 1 //nolint:govet
}

func TestPanicStackTrace(t *testing.T) {
	// Test that when a nil pointer dereference is hit after a
	// `defer AuditPanic()`, the program exits with a non-zero
	// exit code and prints the result (but not stack trace).
	// Inspired by https://go.dev/talks/2014/testing.slide#23
	if os.Getenv("AT_THE_DISCO") == "1" {
		defer AuditPanic()
		testPanicStackTraceHelper()
		return
	}

	// gosec points out that os.Args[0] is tainted, but we only run this as a test
	// so we are not worried about it containing an untrusted value.
	//nolint:gosec
	cmd := exec.Command(os.Args[0], "-test.run=TestPanicStackTrace")
	cmd.Env = append(os.Environ(), "AT_THE_DISCO=1")
	output, err := cmd.CombinedOutput()
	test.AssertError(t, err, "running a failing program")
	test.AssertContains(t, string(output), "nil pointer dereference")
	test.AssertContains(t, string(output), "Stack Trace")
	test.AssertContains(t, string(output), "cmd/shell_test.go:")
}
