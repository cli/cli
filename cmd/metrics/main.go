package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Metrics represents evaluation metrics for the project
type Metrics struct {
	Timestamp     time.Time `json:"timestamp"`
	BuildStatus   string    `json:"build_status"`
	TestPass      int       `json:"test_pass"`
	TestFail      int       `json:"test_fail"`
	Coverage      float64   `json:"coverage"`
	LintIssues    int       `json:"lint_issues"`
	CommandCount  int       `json:"command_count"`
	Dependencies  int       `json:"dependencies"`
	Performance   struct {
		HelpMs int `json:"help_ms"`
	} `json:"performance"`
	Errors []string `json:"errors"`
}

// MetricsCollector collects evaluation metrics
type MetricsCollector struct {
	repoRoot string
	metrics  Metrics
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(repoRoot string) *MetricsCollector {
	return &MetricsCollector{
		repoRoot: repoRoot,
		metrics: Metrics{
			Timestamp: time.Now(),
			Errors:    []string{},
		},
	}
}

// CollectBuildStatus checks if the project builds
func (mc *MetricsCollector) CollectBuildStatus() {
	cmd := exec.Command("go", "build", "-o", "/tmp/gh-metrics-test", filepath.Join(mc.repoRoot, "cmd/gh/main.go"))
	cmd.Dir = mc.repoRoot
	if err := cmd.Run(); err != nil {
		mc.metrics.BuildStatus = "failed"
		mc.metrics.Errors = append(mc.metrics.Errors, fmt.Sprintf("Build error: %v", err))
		return
	}
	os.Remove("/tmp/gh-metrics-test")
	mc.metrics.BuildStatus = "success"
}

// CollectTestMetrics collects test results
func (mc *MetricsCollector) CollectTestMetrics() {
	cmd := exec.Command("go", "test", "-v", "./...")
	cmd.Dir = mc.repoRoot
	output, _ := cmd.CombinedOutput()

	outputStr := string(output)
	mc.metrics.TestPass = strings.Count(outputStr, "PASS:")
	mc.metrics.TestFail = strings.Count(outputStr, "FAIL:")
}

// CollectCoverage collects test coverage metrics
func (mc *MetricsCollector) CollectCoverage() {
	coverFile := filepath.Join(mc.repoRoot, "coverage.out")
	cmd := exec.Command("go", "test", "-v", "-coverprofile="+coverFile, "./...")
	cmd.Dir = mc.repoRoot
	if err := cmd.Run(); err != nil {
		mc.metrics.Errors = append(mc.metrics.Errors, fmt.Sprintf("Coverage collection error: %v", err))
		return
	}

	// Parse coverage
	coverCmd := exec.Command("go", "tool", "cover", "-func="+coverFile)
	coverCmd.Dir = mc.repoRoot
	coverOutput, _ := coverCmd.Output()

	lines := strings.Split(string(coverOutput), "\n")
	for _, line := range lines {
		if strings.Contains(line, "total") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				coverStr := strings.TrimSuffix(parts[2], "%")
				if coverage, err := strconv.ParseFloat(coverStr, 64); err == nil {
					mc.metrics.Coverage = coverage
				}
			}
		}
	}

	os.Remove(coverFile)
}

// CollectLintMetrics collects linting metrics
func (mc *MetricsCollector) CollectLintMetrics() {
	cmd := exec.Command("golangci-lint", "run", "--timeout=5m", "./...")
	cmd.Dir = mc.repoRoot
	output, _ := cmd.CombinedOutput()

	// Count issues (very rough estimate)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if matched, _ := regexp.MatchString(`.*\.go.*`, line); matched {
			mc.metrics.LintIssues++
		}
	}
}

// CollectCommandCount collects the number of commands
func (mc *MetricsCollector) CollectCommandCount() {
	// Build test binary
	cmd := exec.Command("go", "build", "-o", "/tmp/gh-cmd-test", filepath.Join(mc.repoRoot, "cmd/gh/main.go"))
	cmd.Dir = mc.repoRoot
	if err := cmd.Run(); err != nil {
		mc.metrics.Errors = append(mc.metrics.Errors, "Could not collect command count")
		return
	}

	// Count commands
	helpCmd := exec.Command("/tmp/gh-cmd-test", "help")
	output, _ := helpCmd.Output()
	mc.metrics.CommandCount = strings.Count(string(output), "\n") - 1

	os.Remove("/tmp/gh-cmd-test")
}

// CollectDependencies collects dependency count
func (mc *MetricsCollector) CollectDependencies() {
	cmd := exec.Command("go", "list", "-m", "all")
	cmd.Dir = mc.repoRoot
	output, _ := cmd.Output()

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	mc.metrics.Dependencies = len(lines) - 1 // Exclude the main module
}

// CollectPerformanceMetrics collects performance metrics
func (mc *MetricsCollector) CollectPerformanceMetrics() {
	cmd := exec.Command("go", "build", "-o", "/tmp/gh-perf-test", filepath.Join(mc.repoRoot, "cmd/gh/main.go"))
	cmd.Dir = mc.repoRoot
	if err := cmd.Run(); err != nil {
		return
	}

	helpCmd := exec.Command("/tmp/gh-perf-test", "help")
	start := time.Now()
	helpCmd.Output()
	duration := time.Since(start).Milliseconds()
	mc.metrics.Performance.HelpMs = int(duration)

	os.Remove("/tmp/gh-perf-test")
}

// Collect runs all metric collection
func (mc *MetricsCollector) Collect() {
	mc.CollectBuildStatus()
	mc.CollectTestMetrics()
	mc.CollectCoverage()
	mc.CollectLintMetrics()
	mc.CollectCommandCount()
	mc.CollectDependencies()
	mc.CollectPerformanceMetrics()
}

// JSON returns metrics as JSON
func (mc *MetricsCollector) JSON() ([]byte, error) {
	return json.MarshalIndent(mc.metrics, "", "  ")
}

// String returns metrics as a formatted string
func (mc *MetricsCollector) String() string {
	return fmt.Sprintf(`
GitHub CLI Evaluation Metrics
==============================
Timestamp:        %s
Build Status:     %s
Tests Passed:     %d
Tests Failed:     %d
Coverage:         %.2f%%
Lint Issues:      %d
Commands:         %d
Dependencies:     %d
Help Latency:     %dms
Errors:           %v
`,
		mc.metrics.Timestamp.Format(time.RFC3339),
		mc.metrics.BuildStatus,
		mc.metrics.TestPass,
		mc.metrics.TestFail,
		mc.metrics.Coverage,
		mc.metrics.LintIssues,
		mc.metrics.CommandCount,
		mc.metrics.Dependencies,
		mc.metrics.Performance.HelpMs,
		mc.metrics.Errors,
	)
}

func main() {
	var (
		repoRoot = flag.String("repo", ".", "Repository root path")
		format   = flag.String("format", "text", "Output format (text, json)")
		output   = flag.String("output", "", "Output file (empty for stdout)")
	)
	flag.Parse()

	// Get absolute path
	absPath, err := filepath.Abs(*repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Collect metrics
	collector := NewMetricsCollector(absPath)
	collector.Collect()

	// Format output
	var result string
	if *format == "json" {
		data, err := collector.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		result = string(data)
	} else {
		result = collector.String()
	}

	// Write output
	if *output != "" {
		if err := os.WriteFile(*output, []byte(result), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Metrics written to: %s\n", *output)
	} else {
		fmt.Print(result)
	}
}
