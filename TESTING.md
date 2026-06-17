# Testing Guide

Testing guidelines for the PagerDuty Operator.

## Framework

- **Standard `testing` package**: Built-in Go test runner
- **testify/assert**: Assertions and test helpers (`github.com/stretchr/testify/assert`)
- **GoMock**: Interface mocking (`go.uber.org/mock/gomock`)
- **controller-runtime fake client**: Kubernetes API simulation for controller tests

## Quick Commands

```bash
# Run all tests
make go-test

# Run specific package
go test ./controllers/pagerdutyintegration/

# Verbose output
go test -v ./controllers/pagerdutyintegration/

# Run specific test by name
go test -run TestReconcile ./controllers/pagerdutyintegration/

# Race detector
go test -race ./...

# Container-based (CI parity)
boilerplate/_lib/container-make go-test
```

## Writing Tests

### Test Structure

Tests live alongside source code in `*_test.go` files.

**Example:**
```go
package pagerdutyintegration

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "go.uber.org/mock/gomock"
)

func TestMyFeature(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Arrange
    mockClient := NewMockClient(ctrl)
    mockClient.EXPECT().SomeMethod().Return(expectedValue, nil)

    // Act
    result, err := MyFunction(mockClient)

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, expectedValue, result)
}
```

### Controller Tests

Controller tests use `controller-runtime/pkg/client/fake` to simulate the Kubernetes API:

```go
func TestReconcile(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Create fake Kubernetes client with initial objects
    fakeClient := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(existingResource).
        Build()

    // Create mock PagerDuty client
    mockPD := pd.NewMockClient(ctrl)
    mockPD.EXPECT().GetService(gomock.Any()).Return(&serviceObj, nil)

    reconciler := &PagerDutyIntegrationReconciler{
        Client:   fakeClient,
        pdClient: mockPD,
    }

    req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "test"}}
    _, err := reconciler.Reconcile(context.Background(), req)
    assert.NoError(t, err)
}
```

### Mocking Interfaces

GoMock is used for mocking the PagerDuty API client:

```go
// Mock is generated from service.go interface
// Location: pkg/pagerduty/mock_service.go
//
// Regenerate with:
//   boilerplate/_lib/container-make generate
//
// Or directly:
//   mockgen -source=pkg/pagerduty/service.go -destination=pkg/pagerduty/mock_service.go -package=pagerduty
```

**Mock usage:**
```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockPD := pd.NewMockClient(ctrl)
mockPD.EXPECT().CreateService(gomock.Any()).Return("SVC123", nil)
mockPD.EXPECT().DeleteService("SVC123").Return(nil)
```

**Regenerate mocks:**
```bash
boilerplate/_lib/container-make generate
```

## Test Organization

### Unit Tests
- Test individual functions and methods
- Mock external dependencies (PagerDuty API, Kubernetes client)
- Fast execution (<1s per package)
- Located alongside source code

### Controller Tests
- Test reconciliation logic end-to-end
- Use fake Kubernetes client (`controller-runtime/pkg/client/fake`)
- Mock PagerDuty client with GoMock
- Located in `controllers/pagerdutyintegration/`

## Agent-Driven Validation

When AI agents modify code:

**Minimal validation:**
```bash
# After changing controllers/pagerdutyintegration/
go test ./controllers/pagerdutyintegration/
```

**Full validation before commit:**
```bash
make go-test
```

**If tests fail:**
1. Read test output carefully
2. Fix the underlying issue (don't skip tests)
3. Rerun to confirm fix
4. Regenerate mocks if interface changed: `boilerplate/_lib/container-make generate`

## Common Patterns

### Testing with Table-Driven Tests

```go
func TestCreateService(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantErr   bool
    }{
        {"valid service", "my-service", false},
        {"empty name", "", true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()

            mockPD := pd.NewMockClient(ctrl)
            if !tc.wantErr {
                mockPD.EXPECT().CreateService(tc.input).Return("SVC123", nil)
            }

            err := doSomething(mockPD, tc.input)
            if tc.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Testing Error Conditions

```go
func TestReconcileNotFound(t *testing.T) {
    fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

    reconciler := &PagerDutyIntegrationReconciler{Client: fakeClient}

    req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent"}}
    _, err := reconciler.Reconcile(context.Background(), req)
    assert.NoError(t, err) // Not found is not an error
}
```

### Using Assertions

```go
// Equality
assert.Equal(t, expected, actual)

// Nil checks
assert.NoError(t, err)
assert.Nil(t, obj)
assert.NotNil(t, obj)

// Collections
assert.Contains(t, slice, "item")
assert.Len(t, slice, 3)
assert.Empty(t, slice)

// Booleans
assert.True(t, condition)
assert.False(t, condition)

// Require (stops test immediately on failure)
require.NoError(t, err)
```

## Coverage

Generate coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

**Note**: Aim for meaningful coverage, not arbitrary percentages.
- Test critical paths and error handling
- Don't test generated code or trivial getters/setters

## Debugging Tests

```bash
# Verbose output
go test -v ./...

# Print statements in tests
t.Logf("Debug: %v", value)

# Run single test
go test -run "^TestExactName$" ./controllers/pagerdutyintegration/

# Skip slow tests during development
go test -short ./...
```

## CI Expectations

Tests run in Tekton pipeline with:
- Fresh environment
- No cached dependencies
- Strict timeout limits

**Local CI parity:**
```bash
boilerplate/_lib/container-make go-test
```

## Test Performance

**Target timings:**
- Unit tests: <5s per package
- Controller tests: <15s per controller
- Full suite: <2min

**If tests are slow:**
- Check for unnecessary sleeps
- Mock external calls
- Avoid creating unnecessary Kubernetes resources

## Common Issues

**Mock not found:**
```bash
# Regenerate mocks
boilerplate/_lib/container-make generate
```

**Test passes locally, fails in CI:**
```bash
# Run in container environment
boilerplate/_lib/container-make go-test

# Check for:
# - Time-dependent tests
# - Environment-specific assumptions
# - File path dependencies
```

## Further Reading

- [Testify Documentation](https://github.com/stretchr/testify)
- [GoMock Guide](https://pkg.go.dev/go.uber.org/mock/gomock)
- [controller-runtime Testing](https://book.kubebuilder.io/reference/testing.html)
