# Contributing to Deckhouse Helm Generator

Thank you for your interest in contributing!

## Getting Started

### Prerequisites

- Go 1.22+
- Make
- Helm 3.x (for E2E tests)

### Setup

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
make test
```

## Development Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint` (or `golangci-lint run`)
6. Commit with a descriptive message
7. Push and open a Pull Request

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- All exported functions must have comments
- Use table-driven tests
- Handle all errors (linter enforces `errcheck`)

## Project Structure

```
cmd/dhg/          # CLI entry point
pkg/
  analyzer/       # Relationship detection between K8s resources
  extractor/      # Extract resources from YAML/directories
  generator/      # Helm chart generation
  helm/           # Helm chart model and rendering
  processor/      # Per-resource-type processing (k8s/, deckhouse/)
tests/
  e2e/            # End-to-end tests with Helm lint
  integration/    # Integration tests for pipelines
```

## Adding a New Processor

1. Create `pkg/processor/k8s/<resource>.go` (or `deckhouse/`)
2. Implement the `Processor` interface
3. Register in `pkg/processor/registry.go`
4. Add unit tests in `<resource>_test.go`
5. Add test fixtures in `pkg/testutil/fixtures/` if needed

## Testing

```bash
make test              # Unit tests
make test-integration  # Integration tests
make test-e2e          # End-to-end tests
make test-all          # All tests
make coverage          # Coverage report
```

## Pull Request Guidelines

- Keep PRs focused on a single change
- Fill in the PR template completely
- Ensure all CI checks pass
- Update documentation if behavior changes
- Add tests for new functionality

## Commit Messages

Follow conventional commits:

```
feat: add support for Argo Rollouts
fix: handle empty ConfigMap data field
docs: update processor development guide
chore: bump golangci-lint to v1.62
test: add edge cases for Secret processor
```

## Reporting Issues

- Use [Bug Report](.github/ISSUE_TEMPLATE/bug_report.md) for bugs
- Use [Feature Request](.github/ISSUE_TEMPLATE/feature_request.md) for features
- Check existing issues before creating new ones

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
