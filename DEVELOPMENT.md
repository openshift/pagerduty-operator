# Development Guide

Quick reference for developing the PagerDuty Operator.

## Prerequisites

- **Go**: 1.26 or later
- **kubectl**: For cluster interaction
- **prek**: `uv tool install prek`

## Initial Setup

```bash
# Clone repository
git clone https://github.com/openshift/pagerduty-operator.git
cd pagerduty-operator

# Install prek pre-commit hooks
prek install
```

## Common Commands

### Build
```bash
make go-build                 # Build operator binary
make docker-build             # Build container image
```

### Test
```bash
make go-test                  # Run all unit tests
go test ./controllers/pagerdutyintegration/...  # Test specific package
go test -v -run TestReconcile ./controllers/pagerdutyintegration/  # Run specific test
```

### Lint
```bash
make go-check                 # Full linting (golangci-lint)
prek run --all-files          # Run all prek hooks
prek run golangci-lint        # Lint only
```

### Code Generation
```bash
# After modifying API types (api/v1alpha1/*.go)
# or interfaces requiring mocks
boilerplate/_lib/container-make generate

# What this generates:
# - Deepcopy methods (zz_generated.deepcopy.go)
# - OpenAPI schemas
# - Mock interfaces for testing (pkg/pagerduty/mock_service.go)
```

### Container-based Build
```bash
# Run make targets inside boilerplate container
# (ensures consistent environment with CI)
boilerplate/_lib/container-make
boilerplate/_lib/container-make go-test
boilerplate/_lib/container-make generate
```

## Fast Local Iteration

**Minimal validation loop:**
```bash
# After code changes
go build ./...                # Fast compile check (~5s)
go test ./pkg/pagerduty/...   # Run affected tests
prek run                      # Lint staged files
```

**Full validation (pre-PR):**
```bash
prek run --all-files          # All hooks (~15-30s)
make go-test                  # Full test suite
```

## Targeted Testing

```bash
# Run specific test by name
go test -v -run TestMyFunction ./controllers/pagerdutyintegration/

# Run tests for one package
go test -v ./controllers/pagerdutyintegration/

# Run tests with race detector
go test -race ./...
```

## Debugging

```bash
# Print specific package logs
go test -v ./pkg/... 2>&1 | grep "MyFunction"

# Verbose test output
go test -v ./...
```

## Dependency Management

```bash
# Add new dependency
go get github.com/some/package@v1.2.3

# Update dependency
go get -u github.com/some/package

# Tidy (removes unused, adds missing)
go mod tidy

# Verify checksums
go mod verify
```

**Note**: `go.sum` changes automatically trigger validation in prek.

## Architecture Pointers

- **API Types**: `api/v1alpha1/` - CRD definitions
- **Controllers**: `controllers/pagerdutyintegration/` - Reconciliation logic
- **Business Logic**: `controllers/pagerdutyintegration/` - Resource management
- **Tests**: `*_test.go` alongside source
- **Mocks**: `pkg/pagerduty/mock_service.go` - Generated mock for PagerDuty client
- **Metrics**: `pkg/localmetrics/` - Prometheus metrics

## CI Parity

Local prek hooks mirror Tekton CI checks:
- **go-check** ↔ Tekton lint job
- **go-build** ↔ Compilation in CI
- **gitleaks** ↔ Security scanning

Run `prek run --all-files` before pushing to catch CI failures early.

## Boilerplate Integration

This repo uses Red Hat's standardized boilerplate:
- Centralized Makefiles: `boilerplate/openshift/golang-osd-operator/`
- Standard targets: `go-build`, `go-check`, `go-test`
- Container builds: `boilerplate/_lib/container-make`
- Update boilerplate: `make boilerplate-update`

## Troubleshooting

**Mock generation fails:**
```bash
# Use container-make for consistency with CI
boilerplate/_lib/container-make generate
```

**Prek hook timeout:**
```bash
# macOS: Install GNU timeout
brew install coreutils

# Linux: timeout is built-in
```

**go.sum checksum mismatch:**
```bash
export GOPROXY="https://proxy.golang.org"
go mod tidy
```

**Tests fail locally but pass in CI:**
```bash
# Use container environment
boilerplate/_lib/container-make go-test
```

## Further Reading

- [Testing Guide](./TESTING.md)
- [Contributing Guide](./CONTRIBUTING.md)
- [Operator SDK Docs](https://sdk.operatorframework.io/)
