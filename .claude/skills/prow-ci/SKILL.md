---
name: prow-ci
description: Fetch and analyze OpenShift Prow CI job failures with automated artifact download and failure pattern detection
trigger: prow, prow-ci, /prow-ci, ci results, check ci, analyze ci failure
---

# Prow CI Analysis for PagerDuty Operator

This skill fetches Prow CI job artifacts from Google Cloud Storage and provides automated failure analysis.

## Prerequisites

Before using this skill, verify gcloud CLI is installed:
```bash
which gcloud
```

If not installed, provide instructions from: https://cloud.google.com/sdk/docs/install

**Note**: The `test-platform-results` GCS bucket is publicly accessible - no authentication required.

## Quick Start

```bash
# Check PR status and get Prow job URLs
gh pr checks <PR_NUMBER>

# Analyze a failed job
/prow-ci <prow-job-url>

# Or ask naturally:
"Analyze the lint failure in PR <NUMBER>"
"Check why the validate job failed"
"Show me what broke in the coverage job"
```

## Implementation

When invoked, this skill:

1. **Fetches artifacts** using `fetch_prow_artifacts.py`:
   - Downloads **prowjob.json** (job metadata)
   - Downloads **build-log.txt** (complete build output with all errors)
   - Saves to `.work/prow-artifacts/<build-id>/`

2. **Analyzes failures** using `analyze_failure.py`:
   - Parses build-log.txt for error patterns
   - Detects common failure patterns (lint, build, timeout, OOM)
   - Extracts error messages and stack traces
   - Identifies compilation errors and test failures

3. **Generates report**:
   - Markdown format with failure summary
   - Pattern detection (compilation errors, lint failures, timeouts)
   - Top error messages and failures
   - Actionable failure details

## Usage Instructions

### Step 1: Get Prow Job URL

```bash
# View PR checks to find failed jobs
gh pr checks <PR_NUMBER>

# Or get detailed status
gh pr view <PR_NUMBER> --json statusCheckRollup --jq '.statusCheckRollup[] | select(.state == "FAILURE")'
```

### Step 2: Fetch and Analyze

Run the fetch script from repository root:
```bash
python3 .claude/skills/prow-ci/fetch_prow_artifacts.py "<prow-job-url>" -o .work/prow-artifacts
```

### Step 3: Analyze Failures

```bash
python3 .claude/skills/prow-ci/analyze_failure.py .work/prow-artifacts/<build-id> -f markdown
```

### Step 4: Present Findings

Create a clear summary for the user with:
- Root cause identification
- Detected patterns (lint, build, timeout, etc.)
- Key error messages
- Actionable next steps to fix the issue

## Common Job Names

**Prow CI Jobs** (configured in openshift/release):
- `pull-ci-openshift-pagerduty-operator-master-lint` - Linting checks
- `pull-ci-openshift-pagerduty-operator-master-test` - Unit tests
- `pull-ci-openshift-pagerduty-operator-master-validate` - Validation checks
- `pull-ci-openshift-pagerduty-operator-master-coverage` - Code coverage

**Tekton Pipelines** (configured in `.tekton/`):
- `pagerduty-operator-pull-request` - Main PR pipeline
- `pagerduty-operator-pko-pull-request` - PKO pipeline

## Reproducing CI Failures Locally

```bash
# Unit tests
make go-test

# Linting
make go-check

# Full prek validation
prek run --all-files

# Container builds
make docker-build
```

## References

- [Prow Dashboard](https://prow.ci.openshift.org/)
- [CI Search Tool](https://github.com/openshift/ci-search)
- [OpenShift CI Documentation](https://docs.ci.openshift.org/)
