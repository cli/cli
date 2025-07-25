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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

var (
	validPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": { "http-01": true },
  "identifiers": { "dns": true, "ip": true }
}`)
	invalidPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": { "nonsense": true },
  "identifiers": { "openpgp": true }
}`)
	noChallengesIdentsPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false
}`)
	emptyChallengesIdentsPAConfig = []byte(`{
  "dbConnect": "dummyDBConnect",
  "enforcePolicyWhitelist": false,
  "challenges": {},
  "identifiers": {}
}`)
)

func TestPAConfigUnmarshal(t *testing.T) {
	var pc1 PAConfig
	err := json.Unmarshal(validPAConfig, &pc1)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertNotError(t, pc1.CheckChallenges(), "Flagged valid challenges as bad")
	test.AssertNotError(t, pc1.CheckIdentifiers(), "Flagged valid identifiers as bad")

	var pc2 PAConfig
	err = json.Unmarshal(invalidPAConfig, &pc2)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc2.CheckChallenges(), "Considered invalid challenges as good")
	test.AssertError(t, pc2.CheckIdentifiers(), "Considered invalid identifiers as good")

	var pc3 PAConfig
	err = json.Unmarshal(noChallengesIdentsPAConfig, &pc3)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc3.CheckChallenges(), "Disallow empty challenges map")
	test.AssertNotError(t, pc3.CheckIdentifiers(), "Disallowed empty identifiers map")

	var pc4 PAConfig
	err = json.Unmarshal(emptyChallengesIdentsPAConfig, &pc4)
	test.AssertNotError(t, err, "Failed to unmarshal PAConfig")
	test.AssertError(t, pc4.CheckChallenges(), "Disallow empty challenges map")
	test.AssertNotError(t, pc4.CheckIdentifiers(), "Disallowed empty identifiers map")
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
		GRPC *GRPCClientConfig
		TLS  *TLSConfig
	}
	var c config
	err = ReadConfigFile("../test/config/health-checker.json", &c)
	test.AssertNotError(t, err, "ReadConfigFile(../test/config/health-checker.json) errored")
	test.AssertEquals(t, c.GRPC.Timeout.Duration, 1*time.Second)
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
		VitalValue       string          `yaml:"vitalValue" validate:"required"`
		VoluntarilyVoid  string          `yaml:"voluntarilyVoid"`
		VisciouslyVetted string          `yaml:"visciouslyVetted" validate:"omitempty,endswith=baz"`
		VolatileVagary   config.Duration `yaml:"volatileVagary" validate:"required,lte=120s"`
		VernalVeil       config.Duration `yaml:"vernalVeil" validate:"required"`
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

	// Violates 'lte' tag JSON for config.Duration type.
	cf = loadConfigFile(t, "testdata/3_configDuration_too_darn_big.json")
	defer cf.Close()
	err = ValidateJSONConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'lte'")

	// Violates 'lte' tag JSON for config.Duration type.
	cf = loadConfigFile(t, "testdata/3_configDuration_too_darn_big.json")
	defer cf.Close()
	err = ValidateJSONConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected validation error")
	test.AssertContains(t, err.Error(), "'lte'")

	// Incorrect value for the config.Duration type.
	cf = loadConfigFile(t, "testdata/4_incorrect_data_for_type.json")
	defer cf.Close()
	err = ValidateJSONConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected error")
	test.AssertContains(t, err.Error(), "missing unit in duration")

	// Incorrect value for the config.Duration type.
	cf = loadConfigFile(t, "testdata/4_incorrect_data_for_type.yaml")
	defer cf.Close()
	err = ValidateYAMLConfig(&ConfigValidator{&FooConfig{}, nil}, cf)
	test.AssertError(t, err, "Expected error")
	test.AssertContains(t, err.Error(), "missing unit in duration")
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
	*x = 1 //nolint: govet // Purposeful nil pointer dereference to trigger a panic
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

	cmd := exec.Command(os.Args[0], "-test.run=TestPanicStackTrace")
	cmd.Env = append(os.Environ(), "AT_THE_DISCO=1")
	output, err := cmd.CombinedOutput()
	test.AssertError(t, err, "running a failing program")
	test.AssertContains(t, string(output), "nil pointer dereference")
	test.AssertContains(t, string(output), "Stack Trace")
	test.AssertContains(t, string(output), "cmd/shell_test.go:")
}
