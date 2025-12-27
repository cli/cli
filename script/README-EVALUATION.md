# Evaluation Framework

The GitHub CLI Evaluation Framework provides comprehensive tools and metrics for assessing code quality, performance, and functionality.

## Quick Start

```bash
# Run complete evaluation
make eval

# Collect detailed metrics
go run cmd/metrics/main.go

# Export metrics to JSON
go run cmd/metrics/main.go -format json -output metrics.json
```

## Components

### 1. Evaluation Script (`script/eval`)

Automated bash script that performs 13 quality checks:

- **Build Test**: Verifies the project builds successfully
- **Code Formatting**: Checks all code is properly formatted
- **Go Vet**: Runs static analysis
- **Linting**: Checks for code quality issues (requires golangci-lint)
- **Unit Tests**: Runs all unit tests
- **Test Coverage**: Measures code coverage (target: 70%+)
- **Race Detector**: Detects potential race conditions
- **Acceptance Tests**: Runs integration tests
- **Command Help**: Verifies commands are properly documented
- **Dependencies**: Checks dependency status
- **Security**: Scans for security issues (requires gosec)
- **License Compliance**: Verifies license compliance
- **Performance**: Measures command execution time

**Usage**:
```bash
./script/eval
./script/eval --help
```

**Output**:
```
=== GitHub CLI Evaluation Report ===
✅ Build successful
✅ All code properly formatted
✅ No vet issues found
✅ No linting issues
✅ Unit tests passed
✅ Test coverage: 71.5%
... (continues for all 13 checks)

=== EVALUATION SUMMARY ===
Passed: 13
Warnings: 0
Failed: 0
✅ All evaluations passed!
```

### 2. Metrics Collection Tool (`cmd/metrics/main.go`)

Go program that collects detailed metrics about the project:

**Collected Metrics**:
- Build status (success/failed)
- Test pass/fail counts
- Code coverage percentage
- Lint issues count
- Command count
- Dependency count
- Command performance (help latency)

**Usage**:
```bash
# Text output (default)
go run cmd/metrics/main.go

# JSON output
go run cmd/metrics/main.go -format json

# Save to file
go run cmd/metrics/main.go -format json -output metrics.json

# Custom repository
go run cmd/metrics/main.go -repo /path/to/repo
```

**Output Example**:
```json
{
  "timestamp": "2025-12-26T21:35:00Z",
  "build_status": "success",
  "test_pass": 150,
  "test_fail": 0,
  "coverage": 72.5,
  "lint_issues": 0,
  "command_count": 28,
  "dependencies": 47,
  "performance": {
    "help_ms": 45
  },
  "errors": []
}
```

### 3. Documentation (`docs/evaluation-framework.md`)

Comprehensive guide covering:
- Code quality metrics (coverage, linting, complexity)
- Performance metrics (latency, memory, throughput)
- Functionality metrics (command coverage, API integration)
- Reliability metrics (error handling, edge cases)
- Maintainability metrics (organization, documentation)
- Test quality assessment
- Evaluation dashboard
- Pass/fail criteria

## Integration with Makefile

The evaluation framework is integrated into the project Makefile:

```bash
make eval              # Run evaluation framework
make test              # Run all tests
make acceptance        # Run acceptance tests
make licenses-check    # Check license compliance
```

## Pre-commit Setup

Add to `.git/hooks/pre-commit` for automatic checks:

```bash
#!/bin/bash
set -e
go fmt ./...
go vet ./...
go test -short ./...
./script/eval --quick
```

## Quality Thresholds

| Metric | Pass | Warning | Fail |
|--------|------|---------|------|
| Test Coverage | ≥70% | 50-70% | <50% |
| Build Status | Success | N/A | Failed |
| Test Pass Rate | 100% | 95%+ | <95% |
| Linting Issues | 0 | 1-5 | >5 |
| Complexity | ≤10 | 11-15 | >15 |
| Command Latency | <500ms | 500-2000ms | >2000ms |

## Continuous Integration

The framework is designed to run in CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Run Evaluation
  run: make eval

- name: Collect Metrics
  run: go run cmd/metrics/main.go -format json -output metrics.json

- name: Upload Metrics
  uses: actions/upload-artifact@v2
  with:
    name: evaluation-metrics
    path: metrics.json
```

## Troubleshooting

### Missing Tools

Some checks require additional tools:

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install gosec (security scanner)
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Install gocyclo (complexity analyzer)
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
```

### Skipping Slow Tests

```bash
# Run fast tests only
go test -short ./...

# Run with timeout
go test -timeout=5m ./...
```

### Coverage Report

```bash
# Generate HTML coverage report
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Best Practices

1. **Run before commits**: Use pre-commit hooks to catch issues early
2. **Monitor trends**: Track metrics over time to identify regressions
3. **Set targets**: Establish baselines for coverage, latency, etc.
4. **Review failures**: Investigate why tests fail before fixing
5. **Document changes**: Update documentation when modifying evaluation criteria

## Next Steps

- Read [docs/evaluation-framework.md](../docs/evaluation-framework.md) for detailed metrics
- Review [docs/project-layout.md](../docs/project-layout.md) for architecture
- Check [.github/workflows](../.github/workflows) for CI/CD integration
- See [CONTRIBUTING.md](../CONTRIBUTING.md) for development guidelines

## Related Commands

```bash
make test              # Run all tests
make acceptance        # Run acceptance tests  
make clean             # Clean build artifacts
make licenses-check    # Check license compliance
make manpages          # Generate man pages
make completions       # Generate shell completions
```

## Support

For issues or questions about the evaluation framework:
1. Check [docs/evaluation-framework.md](../docs/evaluation-framework.md)
2. Review test output for specific failures
3. Run `./script/eval --help` for script options
4. Use `go run cmd/metrics/main.go` to see individual metrics
