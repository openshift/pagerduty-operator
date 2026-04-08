# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The PagerDuty operator runs on Hive and automates PagerDuty integration for OpenShift Dedicated clusters. It watches PagerDutyIntegration CRs and matching ClusterDeployments, then creates PagerDuty services, integration keys, and Hive SyncSets to distribute alerting credentials to managed clusters.

## Build & Test Commands

All make targets come from OpenShift boilerplate (`boilerplate/openshift/golang-osd-operator/`). The main Makefile just sets `FIPS_ENABLED=true` and includes generated boilerplate.

```bash
make test                    # Run all unit tests (uses envtest for kubebuilder assets)
make lint                    # golangci-lint + YAML validation
make generate                # CRDs, deepcopy, openapi-gen, go:generate (mockgen)
make validate                # Ensure generated code is committed and unchanged
make go-build                # Build binary (FIPS-enabled, may fail locally - see below)
make coverage                # Code coverage report
make container-test          # Run tests in boilerplate backing container (matches CI)
```

**Running a single test or package:**
```bash
go test ./controllers/pagerdutyintegration/... -v -run TestReconcile
go test ./pkg/pagerduty/... -v
go test ./pkg/pko/... -v
```

**PKO template snapshot tests:**
```bash
kubectl-package validate deploy_pko/        # Compare rendered templates against fixtures
rm -rf deploy_pko/.test-fixtures && kubectl-package validate deploy_pko/  # Regenerate fixtures
```

**FIPS note:** `FIPS_ENABLED=true` sets `GOEXPERIMENT=boringcrypto` which will fail outside the CI container. For local Go builds, use `go build .` directly or use `make container-test` to run in the CI-equivalent container.

## Architecture

### Reconciliation Flow

The single controller (`PagerDutyIntegrationReconciler`) reconciles PagerDutyIntegration CRs:

1. Lists all ClusterDeployments, filters by `spec.clusterDeploymentSelector`
2. For each matching CD with `spec.installed: true`:
   - `handleCreate` - Creates PD service, integration key Secret, SyncSet, and ConfigMap
   - `handleUpdate` - Updates alert grouping parameters if changed
   - `handleServiceOrchestration` - Applies orchestration rules from ConfigMap
   - `handleLimitedSupport` - Enables/disables PD service based on cluster support status

### Key Packages

- `controllers/pagerdutyintegration/` - Reconciler and per-operation handler files (create, delete, update, orchestration, limited support, event handlers)
- `pkg/pagerduty/` - PagerDuty API client wrapping `go-pagerduty`. Has a `Client` interface with mockgen-generated mock (`mock_service.go`)
- `pkg/kube/` - Generates Kubernetes resources (ConfigMaps, SyncSets) for Hive
- `pkg/utils/` - Helpers for secrets, ConfigMaps, finalizers, cluster ID resolution
- `config/` - Constants and environment config (namespace names, label keys, finalizer prefixes)
- `api/v1alpha1/` - PagerDutyIntegration CRD type definition
- `deploy_pko/` - Package Operator deployment templates (`.gotmpl` files)
- `pkg/pko/` - Go tests for PKO template rendering

### Resource Naming

Secondary resources follow the pattern: `{servicePrefix}-{clusterName}{suffix}` where suffix is `-pd-secret` or `-pd-config`.

### Finalizer System

Two-level finalizers control cleanup ordering:
- PDI finalizer: `pd.managed.openshift.io/pagerduty` (on the PagerDutyIntegration CR)
- Per-CD finalizer: `pd.managed.openshift.io/{pdi-name}` (on each ClusterDeployment)

### FedRAMP Mode

Set via `FEDRAMP` env var. Changes how cluster ID is derived: from namespace suffix instead of `spec.clusterName`.

### Custom Event Handlers

The controller uses custom event handlers (`event_handlers.go`) to trigger PDI reconciliation from ClusterDeployment changes, SyncSet/ConfigMap/Secret owner changes, and orchestration ConfigMap changes.

## Code Generation

After modifying CRD types or the PagerDuty client interface:
```bash
make generate        # Regenerates CRDs, deepcopy, openapi, mocks
make generate-check  # Verify nothing is uncommitted (what CI runs)
```

Generated files that must be committed: `zz_generated.deepcopy.go`, `zz_generated.openapi.go`, `mock_service.go`, CRD YAML in `deploy/crds/`.

## CI

- Tekton/Konflux pipelines in `.tekton/`
- Boilerplate CI image version set in `.ci-operator.yaml`
- `container-*` targets run make in the boilerplate backing container to match CI

## Owners

Primary maintainers: clcollins, drow. Approval groups: srep-functional-team-rocket, srep-functional-leads, srep-team-leads.
