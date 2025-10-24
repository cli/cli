# GitHub CLI - Copilot Instructions

This repository contains the GitHub CLI (`gh`), a command-line tool that brings GitHub to your terminal.

## Project Overview

- **Language**: Go 1.24+
- **Type**: Command-line interface tool
- **Purpose**: GitHub CLI for interacting with GitHub from the terminal
- **License**: Open source (MIT)

## Project Structure

- `cmd/` - Main packages for building the `gh` executable
- `pkg/` - Command implementations and core packages
- `internal/` - Internal packages specific to this project
- `api/` - GitHub API client utilities
- `git/` - Git repository interaction utilities
- `docs/` - Documentation for contributors and maintainers
- `script/` - Build and release automation scripts

Command implementation follows this pattern:
```
pkg/cmd/<command>/<subcommand>/<subcommand>.go
```

See [docs/project-layout.md](../docs/project-layout.md) for detailed information.

## Building and Testing

### Building

**Unix-like systems:**
```bash
make                # Build the binary
bin/gh              # Run the built binary
```

**Windows:**
```bash
go run script/build.go
bin\gh
```

### Testing

Run all tests:
```bash
go test ./...
```

Run acceptance tests:
```bash
go test -tags acceptance ./acceptance
```

## Coding Standards

### General Principles

- Follow existing Go conventions and idioms
- Keep changes minimal and focused
- Use existing libraries and patterns already in the codebase
- Write tests for new functionality and bug fixes
- Ensure code passes `go test ./...` before submitting

### Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Keep functions focused and concise
- Add comments only when necessary to explain non-obvious logic
- Update help text when modifying commands

### Commands

- Each command should have clear help text
- Commands use the Cobra framework
- Help text is embedded in the command's source file
- Commands should handle errors gracefully and provide helpful error messages

### API Interactions

- Use the utilities in `api/` package for GitHub API calls
- Handle rate limiting appropriately
- Use GraphQL when possible for efficiency
- Include proper error handling for network failures

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed contribution guidelines.

### Pull Request Guidelines

- Only work on issues labeled `help wanted`
- Issues labeled `core` require internal context and are not open for external contributions
- Keep PR scope limited to the issue's acceptance criteria
- Add tests for new features and bug fixes
- Update documentation if adding/modifying commands
- Mention `@cli/code-reviewers` if acceptance criteria are unclear

### Testing Requirements

- All new code should include tests
- Tests should pass locally before submitting PR
- Acceptance tests should be added for user-facing changes
- Mock external dependencies in unit tests

## Release Process

- Releases follow semantic versioning
- See [docs/releasing.md](../docs/releasing.md) for the release process
- Manual pages are auto-generated from help text during releases
- Binaries are built for macOS, Linux, and Windows

## Additional Resources

- [Project Layout Documentation](../docs/project-layout.md)
- [Command-line Syntax Guidelines](../docs/command-line-syntax.md)
- [Release Process Details](../docs/releasing.md)
- [Security Guidelines](../docs/SECURITY_GUIDE.md)
