# PagerDuty Operator Validation Scripts

Validation scripts for testing pagerduty-operator rollouts across integration, staging, and production hives.

All scripts are **READ-ONLY** — they do not modify any cluster or PagerDuty resources. Secret content is never read or decoded.

## Quick Start

### Prerequisites

- Logged into a hive cluster: `ocm backplane login <hive>`
- A Jira ticket URL for elevation justification
- `jq`, `ocm`, and `oc` CLI tools available

### Running Validations with Make

```bash
# Set your Jira ticket
REASON="https://redhat.atlassian.net/browse/SREP-XXXX"

# Run all validations (deployment + functional + metrics)
make -f scripts/validate.mk validate REASON=$REASON

# Or run individually
make -f scripts/validate.mk validate-deployment REASON=$REASON
make -f scripts/validate.mk validate-functional REASON=$REASON
make -f scripts/validate.mk validate-metrics REASON=$REASON
```

### Baseline Workflow

```bash
# Before promotion: capture baseline on each target hive
ocm backplane login hive-stage-01
make -f scripts/validate.mk baseline REASON=$REASON

# After promotion: compare against baseline
make -f scripts/validate.mk compare REASON=$REASON BASELINE=/tmp/baselines/baseline-hive-stage-01-20260318.json
```

### Make Targets

| Target | Description |
|--------|-------------|
| `help` | Show available targets and usage |
| `baseline` | Capture metrics baseline (saves to `/tmp/baselines/`) |
| `compare` | Compare current metrics against a baseline file (`BASELINE=<file>` required) |
| `validate` | Run all three validations in sequence |
| `validate-deployment` | Deployment health only |
| `validate-functional` | Functional behavior only |
| `validate-metrics` | Prometheus metrics only |

All targets require `REASON=<jira-ticket-url>`. Optional variables: `NAMESPACE` (default: `pagerduty-operator`), `BASELINE_DIR` (default: `/tmp/baselines`), `VERBOSE` (default: `-v`).

## Validating a Rollout

### 1. Capture Baselines (Before Promotion)

On each target hive, capture a metrics snapshot:

```bash
ocm backplane login <hive>
make -f scripts/validate.mk baseline REASON=$REASON
```

Repeat for all target hives. Baselines are saved to `/tmp/baselines/baseline-<hive>-<date>.json`.

### 2. After Promotion, Validate Each Hive

```bash
ocm backplane login <hive>
make -f scripts/validate.mk validate REASON=$REASON
```

This runs deployment, functional, and metrics checks in sequence.

### 3. Compare Against Baselines

```bash
make -f scripts/validate.mk compare REASON=$REASON BASELINE=/tmp/baselines/baseline-<hive>-<date>.json
```

### 4. Progression Order

1. **Integration** (1 hive) — validate, capture baseline
2. **Staging** (3 hives) — may auto-promote; validate all 3
3. **Production** (8 hives) — validate a representative sample

If `validate-functional` reports "no new clusters since rollout", wait for cluster provisioning or re-run later.

## Script Details

### validate-deployment.sh

**Purpose**: Is the operator pod running and healthy?

**Checks**:
- Cluster connection verified
- Operator namespace exists
- Pod is `Running` with 0 restarts
- Pod image/version reported
- Error-level logs scanned (benign "namespace terminating" errors filtered separately)
- Deployment has desired replicas available and ready

**Elevation**: All `oc` commands go through `ocm backplane elevate` using the provided ticket.

**Usage**:
```bash
./validate-deployment.sh -t <ticket-url> [-v] [-j] [-n namespace]
```

### validate-functional.sh

**Purpose**: Is the operator actively creating PagerDuty services for new clusters?

**Checks**:
1. **PagerDutyIntegration CRs** — verifies they exist and have active finalizers; collects service prefixes used for resource naming
2. **Operator rollout time** — determines when the current pod started (i.e., when the new version began running)
3. **New ClusterDeployments** — finds clusters installed *after* the operator rollout, proving the current version handled them
4. **Spot-check PD resources** — selects up to 3 representative new clusters (first, middle, last by install time) and validates each with a single bulk API call (`oc get configmap,secret,syncset -n <ns>`):
   - **ConfigMap** (`{prefix}-{cdName}-pd-config`): has `SERVICE_ID`, `INTEGRATION_ID`, `ESCALATION_POLICY_ID`
   - **Secret** (`{prefix}-{cdName}-pd-secret`): exists (content is NOT read or decoded)
   - **SyncSet** (`{prefix}-{cdName}-pd-secret`): references correct ClusterDeployment, `resourceApplyMode: Sync`, valid `secretMappings` with source and target refs
   - **PD finalizer**: present on the ClusterDeployment
5. **PD finalizer coverage** — checks all new eligible CDs have PD finalizers (filters out fake, nightly, and noalerts clusters which are excluded by design)
6. **Log activity** — heartbeat (PD API ping every 5 min), successful reconciliations, error classification
7. **Creation evidence** — searches logs for `creating pd secret` entries for new cluster namespaces

**No new clusters?** If no clusters were provisioned since the operator rollout, the script reports clearly and suggests waiting or requesting a test cluster.

**Resource scoping**: All resource lookups are scoped to namespaces where ClusterDeployments actually exist — no `--all-namespaces` sweeps for ConfigMaps, Secrets, or SyncSets.

**Usage**:
```bash
./validate-functional.sh -t <ticket-url> [-v] [-j] [-n namespace]
```

### check-metrics.sh

**Purpose**: Are the operator's Prometheus metrics healthy?

**Checks**:
- `pagerdutyintegration_secret_loaded == 1` (PD API key loaded)
- `pagerduty_create_failure == 0` (no service creation failures)
- `pagerduty_delete_failure == 0` (no service deletion failures)
- `pagerduty_heartbeat_count > 0` (PD API reachable)
- `pagerduty_operator_reconcile_duration_seconds` p95/p99 within bounds

**How it works**: Queries the app-sre Prometheus instance in `openshift-customer-monitoring` via `ocm backplane elevate ... exec` into the Prometheus pod using `wget`.

**Usage**:
```bash
./check-metrics.sh -t <ticket-url> [-v] [-j] [-n namespace]
```

### compare-metrics.sh

**Purpose**: Has anything regressed compared to a pre-promotion baseline?

**Modes**:
- `--baseline`: Captures a metrics snapshot as JSON (pipe to a file)
- `--compare <file>`: Compares current metrics against a saved baseline

**Metrics compared**:
- Secret loaded status
- Create/delete failure counts
- Heartbeat count
- Reconciliation latency (p50, p95, p99) with % change
- API request latency with % change
- Pod CPU and memory usage with % change

**Thresholds**: Warns at >50% latency increase, fails at >100%. Warns at >50% memory increase.

**Usage**:
```bash
# Capture
./compare-metrics.sh --baseline -t <ticket-url> > baseline.json

# Compare
./compare-metrics.sh --compare baseline.json -t <ticket-url> [-v]
```

### validation-suite.sh

**Purpose**: Orchestrates all validation checks in sequence.

**Note**: The `make validate` target provides equivalent functionality and is the recommended way to run the full suite.

## Output

### Standard Output

Color-coded, human-readable:
- ✅ Green — check passed
- ❌ Red — check failed
- ⚠️  Yellow — warning (non-critical issue)
- 🔵 Blue — action needed (e.g., no new clusters to validate)

### JSON Output

Use `-j` flag for machine-readable output. All scripts support this.

### Exit Codes

- `0` — all checks passed (or passed with warnings)
- `1` — one or more checks failed

## Troubleshooting

### "REASON is required"
Set `REASON` to a Jira ticket URL: `REASON=https://redhat.atlassian.net/browse/SREP-XXXX`

### "Not logged into a cluster"
Run `ocm backplane login <hive>` first.

### "Backplane elevation failed"
Verify the Jira ticket URL is correct and grants appropriate access.

### "No PD API heartbeat found"
The operator may have just started. Wait 5 minutes for the first heartbeat cycle.

### "No new clusters to validate"
The functional check only validates clusters installed after the operator rollout. If no clusters have been provisioned, wait for provisioning activity or re-run later.

### Resource count mismatches
ConfigMap/Secret/SyncSet counts should be equal. Mismatches may indicate in-progress reconciliation — re-run after a few minutes.
