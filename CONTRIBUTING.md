# Contributing to Intentra CLI

Thank you for your interest in contributing to Intentra CLI! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## How to Contribute

### Reporting Bugs

Before creating a bug report, please check existing issues to avoid duplicates. When creating a bug report, include:

- A clear, descriptive title
- Steps to reproduce the issue
- Expected vs actual behavior
- Your environment (OS, Go version, AI tool versions)
- Relevant logs or error messages

### Suggesting Features

Feature requests are welcome! Please:

- Use a clear, descriptive title
- Explain the use case and why existing features don't meet your needs
- Provide examples of how the feature would work

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Make your changes** following our coding standards
3. **Add tests** for any new functionality
4. **Ensure all tests pass** with `make test`
5. **Run the linter** with `make lint`
6. **Update documentation** if needed
7. **Submit a pull request**

## Development Setup

### Prerequisites

- Go 1.21 or later
- Make (optional but recommended)

### Getting Started

```bash
git clone https://github.com/intentrahq/intentra-cli.git
cd intentra-cli
make build
make test
```

### Project Structure

```
intentra-cli/
├── cmd/intentra/           # CLI entry point and commands
├── internal/
│   ├── api/                # HTTP client for server communication
│   ├── auth/               # Authentication and token management
│   ├── config/             # Configuration management
│   ├── device/             # Device identification
│   ├── hooks/              # Hook management and event normalization
│   │   ├── handler.go      # Event processing and scan aggregation
│   │   ├── manager.go      # Hook installation/uninstallation
│   │   ├── normalizer.go   # Normalizer interface and constants
│   │   ├── normalizer_cursor.go    # Cursor event mappings
│   │   ├── normalizer_claude.go    # Claude Code event mappings
│   │   ├── normalizer_gemini.go    # Gemini CLI event mappings
│   │   ├── normalizer_copilot.go   # GitHub Copilot event mappings
│   │   ├── normalizer_windsurf.go  # Windsurf Cascade event mappings
│   │   └── templates.go    # Hook JSON templates per tool
│   └── scanner/            # Scan aggregation and tracking
└── pkg/models/             # Data models (Event, Scan)
```

### Adding Support for a New Tool

1. Create `internal/hooks/normalizer_<tool>.go` implementing the `Normalizer` interface
2. Register the normalizer in `init()` with `RegisterNormalizer()`
3. Add tool constants to `internal/hooks/manager.go`
4. Add hook JSON template to `internal/hooks/templates.go`
5. Add tests in `internal/hooks/*_test.go`
6. Update documentation

## Coding Standards

### Go Style

- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use `gofmt` for formatting (enforced by CI)
- Run `golangci-lint` before submitting

### Code Organization

- Keep functions focused and small
- Use meaningful variable and function names
- Add docstrings to all exported functions and types

### Testing

- Write unit tests for new functionality
- Maintain or improve code coverage
- Use table-driven tests where appropriate

### Commit Messages

Use clear, descriptive commit messages:

```
feat: add support for new AI tool XYZ

- Implement normalizer for XYZ event format
- Add hook installation for XYZ config location
- Update documentation
```

Prefixes:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `test:` Test additions or changes
- `refactor:` Code refactoring
- `chore:` Maintenance tasks

### Branch Naming

Use descriptive branch names:
- `feature/add-xyz-support`
- `fix/cursor-hook-path`
- `docs/update-readme`

## Testing

### Running Tests

```bash
make test
```

### Running Specific Tests

```bash
go test ./internal/hooks/... -v
```

### Test Coverage

```bash
make coverage
```

## Documentation

- Update README.md for user-facing changes
- Add/update code comments for complex logic
- Include examples for new features

## Release Process

Releases are automated via GitHub Actions when a new tag is pushed:

1. Update CHANGELOG.md with release notes
2. Create and push a version tag: `git tag v1.2.3 && git push origin v1.2.3`
3. GitHub Actions builds and publishes release artifacts

## Getting Help

- Open a [GitHub Issue](https://github.com/intentrahq/intentra-cli/issues) for bugs or feature requests
- Start a [Discussion](https://github.com/intentrahq/intentra-cli/discussions) for questions

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
