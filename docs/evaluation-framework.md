# Evaluation Framework for GitHub CLI

This document defines the evaluation framework for assessing the quality, performance, and functionality of the GitHub CLI codebase.

## Overview

The evaluation framework provides metrics and tools to measure:
- **Code Quality**: Test coverage, linting, and static analysis
- **Performance**: Execution time, memory usage, and throughput
- **Functionality**: Feature completeness and command coverage
- **Reliability**: Error handling and edge case handling
- **Maintainability**: Code complexity and documentation

## 1. Code Quality Metrics

### 1.1 Test Coverage

**Objective**: Ensure adequate test coverage across the codebase.

**Targets**:
- Minimum 70% coverage for all packages
- 90%+ coverage for critical paths (auth, API, repo operations)
- 100% coverage for error handling logic

**Measurement**:
```bash
# Generate coverage report
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Check coverage by package
go test -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -E "ok|fail"
```

**Thresholds**:
- ✅ **Pass**: Coverage ≥ 70%
- ⚠️ **Warning**: Coverage 50-70%
- ❌ **Fail**: Coverage < 50%

### 1.2 Linting & Static Analysis

**Objective**: Maintain code quality standards.

**Tools**:
- `golangci-lint`: Configuration in `.golangci.yml`
- `go vet`: Built-in Go analyzer
- `go fmt`: Code formatting

**Execution**:
```bash
# Run linters
golangci-lint run ./...

# Check formatting
go fmt ./...

# Run go vet
go vet ./...
```

**Categories to Monitor**:
- Unused variables and imports
- Shadowed variables
- Incorrect error handling
- Security issues
- Performance issues

### 1.3 Cyclomatic Complexity

**Objective**: Keep functions simple and maintainable.

**Thresholds**:
- ✅ **Pass**: Complexity ≤ 10 per function
- ⚠️ **Warning**: Complexity 11-15
- ❌ **Fail**: Complexity > 15

**Measurement**:
```bash
go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
gocyclo -over 10 ./pkg ./cmd ./internal
```

## 2. Performance Metrics

### 2.1 Benchmarking

**Objective**: Track performance regressions and identify bottlenecks.

**Test Pattern**:
```go
func BenchmarkCreatePullRequest(b *testing.B) {
    for i := 0; i < b.N; i++ {
        // benchmark code
    }
}
```

**Execution**:
```bash
# Run benchmarks
go test -bench=. -benchmem ./pkg/cmd/pr/create

# Compare benchmark results
go test -bench=. -benchmem -benchstat ./pkg/cmd/pr/create
```

**Key Metrics**:
- **Latency**: Command execution time (target: < 1s for most commands)
- **Memory**: Allocation counts and bytes (minimize GC pressure)
- **Throughput**: Operations per second

### 2.2 Command Execution Time

**Objective**: Monitor CLI responsiveness.

**Thresholds**:
- ✅ **Pass**: < 500ms for interactive commands
- ⚠️ **Warning**: 500ms - 2s
- ❌ **Fail**: > 2s (without valid reason)

**Measurement**:
```bash
time gh pr list
time gh issue search "state:open"
time gh repo view
```

### 2.3 Memory Footprint

**Objective**: Minimize memory usage for resource-constrained environments.

**Baseline**: < 50MB for typical operations

**Profiling**:
```go
import _ "net/http/pprof"

// Enable in tests:
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./pkg/cmd/pr/list
go tool pprof mem.prof
```

## 3. Functionality Metrics

### 3.1 Command Coverage

**Objective**: Track implementation status of all planned commands.

**Inventory**: 25+ top-level commands

**Status Tracking**:
- List all commands: `gh help`
- Verify each command has proper help text
- Check subcommands are properly registered

**Completeness Checklist**:
- [ ] Help text present and accurate
- [ ] Examples documented
- [ ] All flags working
- [ ] Error messages clear

### 3.2 API Integration Coverage

**Objective**: Ensure API endpoints are properly wrapped.

**Measurement**:
```bash
# Count GraphQL queries
find ./api -name "queries_*.go" | wc -l

# List all mutation types
grep -r "type.*Mutation" ./api
```

**Targets**:
- GraphQL queries for common operations
- REST endpoints for unsupported GraphQL operations
- Proper error handling for API failures

### 3.3 Feature Completeness

**Objective**: Verify features match upstream requirements.

**Review Criteria**:
- GitHub feature → gh command mapping
- Feature parity with web interface (where applicable)
- Enterprise Server support

## 4. Reliability Metrics

### 4.1 Error Handling

**Objective**: Comprehensive error coverage and user-friendly messages.

**Assessment**:
- Exit codes properly used (0=success, 1=error, 2=cancel, 4=auth, 8=pending)
- Error messages are actionable
- Network failures handled gracefully
- Invalid input detected early

**Test Categories**:
- Authentication failures
- Network timeouts
- Invalid arguments
- Rate limiting
- Malformed responses

### 4.2 Edge Case Handling

**Objective**: Handle unusual but valid inputs.

**Test Scenarios**:
- Empty results
- Large result sets (pagination)
- Special characters in names
- Unicode in descriptions
- Enterprise Server URLs
- Multi-account scenarios

### 4.3 Integration Testing

**Objective**: Verify end-to-end workflows.

**Acceptance Tests** (tagged with `acceptance`):
```bash
go test -tags acceptance ./acceptance
```

**Test Scope**:
- Authentication flows
- Repository operations
- Pull request workflows
- Issue management
- Organization operations

## 5. Maintainability Metrics

### 5.1 Code Organization

**Objective**: Maintain clear separation of concerns.

**Structure Assessment**:
```
pkg/cmd/<domain>/<action>/
├── <action>.go           (command definition + run logic)
├── <action>_test.go      (tests)
└── shared/               (domain-specific utilities)

internal/                 (internal packages)
api/                      (API client wrapper)
git/                      (Git operations)
```

**Checklist**:
- [ ] One responsibility per package
- [ ] No circular dependencies
- [ ] Clear public/private APIs
- [ ] Documentation for exported types

### 5.2 Documentation Quality

**Objective**: Comprehensive and accurate documentation.

**Elements**:
- [ ] Command help text (`.Short`, `.Long`)
- [ ] Code comments for complex logic
- [ ] Examples in help text (`.Example`)
- [ ] Architecture documentation
- [ ] Testing patterns documented

### 5.3 Dependency Management

**Objective**: Keep dependencies minimal and well-maintained.

**Metrics**:
- Total dependencies (minimize)
- Dependency freshness (keep updated)
- License compliance (review quarterly)

**Verification**:
```bash
go list -u ./...          # Check for updates
./script/licenses         # Check license compliance
go mod why <dep>          # Understand dependency
```

## 6. Test Quality Assessment

### 6.1 Test Types Distribution

**Target Distribution**:
- Unit tests: 70% (fast, isolated)
- Integration tests: 20% (multiple components)
- Acceptance tests: 10% (end-to-end)

**Metrics**:
```bash
# Count test types
find . -name "*_test.go" | wc -l
grep -r "func Test" ./pkg ./internal --include="*_test.go" | wc -l
grep -r "func Benchmark" --include="*_test.go" | wc -l
```

### 6.2 Test Isolation

**Objective**: Tests should not depend on external state.

**Assessment**:
- ✅ Uses `httpmock.Registry` for HTTP mocking
- ✅ Uses `iostreams.Test()` for I/O
- ✅ No real filesystem access
- ✅ No real GitHub API calls
- ✅ No hardcoded paths

### 6.3 Test Data Management

**Objective**: Fixtures are realistic and well-organized.

**Structure**:
```
pkg/cmd/pr/list/
└── fixtures/
    ├── prList.json
    ├── prListFiltered.json
    └── README.md (documents fixture contents)
```

**Validation**:
- Fixtures match actual API response format
- Fixtures cover happy path and edge cases
- Fixtures are updated when API changes

## 7. Evaluation Dashboard

### Quick Status Check

Run this script to get a comprehensive evaluation report:

```bash
#!/bin/bash
# eval-report.sh

echo "=== GitHub CLI Evaluation Report ==="
echo

echo "1. Test Coverage"
go test -v -coverprofile=coverage.out ./... 2>/dev/null
COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}')
echo "Overall Coverage: $COVERAGE"
echo

echo "2. Linting"
golangci-lint run --deadline=5m ./... 2>&1 | tail -5
echo

echo "3. Build"
go build -o bin/gh cmd/gh/main.go && echo "✅ Build successful" || echo "❌ Build failed"
echo

echo "4. Tests"
go test -timeout=10m ./... && echo "✅ All tests passed" || echo "❌ Some tests failed"
echo

echo "5. Acceptance Tests"
go test -tags acceptance ./acceptance && echo "✅ Acceptance tests passed" || echo "❌ Acceptance tests failed"
```

## 8. Continuous Evaluation

### 8.1 Pre-commit Checks

**Recommended hooks** (.git/hooks/pre-commit):
```bash
#!/bin/bash
go fmt ./...
go vet ./...
go test -short ./...
```

### 8.2 CI/CD Pipeline

**GitHub Actions** (in `.github/workflows/`):
- Run linters on every PR
- Run test suite
- Generate coverage reports
- Run acceptance tests
- Check license compliance
- Verify builds on multiple platforms

### 8.3 Regular Reviews

**Weekly**:
- Test coverage trends
- Performance benchmarks
- New linting issues

**Monthly**:
- Dependency updates
- Architecture review
- Documentation accuracy

**Quarterly**:
- License compliance audit
- Security audit
- Major refactoring assessment

## 9. Pass/Fail Criteria

A workspace is considered **passing evaluation** when:

| Metric | Threshold | Status |
|--------|-----------|--------|
| Test Coverage | ≥ 70% | ✅ |
| Linting Issues | 0 critical | ✅ |
| Cyclomatic Complexity | ≤ 10 per function | ✅ |
| Build Status | Passes on all platforms | ✅ |
| Test Status | 100% pass rate | ✅ |
| Command Latency | < 500ms (average) | ✅ |
| Memory Usage | < 50MB (typical) | ✅ |
| Documentation | Complete and accurate | ✅ |

## 10. Tools & Resources

### Helpful Commands

```bash
# Full evaluation
make test                          # Run all tests
make acceptance                    # Run acceptance tests
go test -cover ./...              # With coverage
go test -v -race ./...            # With race detector
golangci-lint run ./...           # Linting
go mod graph                       # Dependency visualization
gh help                            # List all commands
```

### External Tools

- **Coverage**: `golang.org/x/tools/cmd/cover`
- **Linting**: `github.com/golangci/golangci-lint`
- **Complexity**: `github.com/fzipp/gocyclo`
- **Profiling**: Go's built-in `pprof`
- **Benchmarking**: `golang.org/x/perf/cmd/benchstat`

## 11. Evaluation Workflow

### For Contributors

1. **Before submitting PR**:
   - Run `make test` and verify all tests pass
   - Run `golangci-lint run ./...` and fix any issues
   - Add tests for new code (target: 70%+ coverage)
   - Update documentation

2. **During code review**:
   - Verify test coverage for changes
   - Check cyclomatic complexity
   - Validate error handling
   - Confirm documentation is updated

3. **Post-merge monitoring**:
   - CI/CD pipeline validation
   - Performance regression detection
   - Acceptance test coverage

### For Maintainers

1. **Weekly**: Review coverage trends, new issues
2. **Monthly**: Dependency audits, security reviews
3. **Quarterly**: Architecture assessment, major refactoring
4. **Annually**: Comprehensive audit of all metrics

## 12. Common Evaluation Scenarios

### Scenario 1: Adding a New Command

**Evaluation checklist**:
- [ ] Command implementation follows architecture patterns
- [ ] Tests cover happy path and error cases (70%+ coverage)
- [ ] Help text is clear and includes examples
- [ ] All flags documented and tested
- [ ] Integration with Factory/IOStreams patterns
- [ ] No linting errors
- [ ] Performance acceptable (< 500ms)

### Scenario 2: Refactoring Large Component

**Evaluation criteria**:
- [ ] Test suite still passes (100%)
- [ ] Code coverage maintained or improved
- [ ] Cyclomatic complexity reduced or maintained
- [ ] No performance regressions
- [ ] Documentation updated

### Scenario 3: Performance Optimization

**Evaluation process**:
- [ ] Baseline benchmarks established
- [ ] Optimization implemented
- [ ] Benchmarks show improvement
- [ ] No functional regressions
- [ ] Memory usage acceptable

## References

- [Project Layout Documentation](./project-layout.md)
- [Testing Patterns](../pkg/cmd/issue/list/list_test.go)
- [GitHub Workflows](./.github/workflows/)
