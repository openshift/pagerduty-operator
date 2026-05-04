#!/bin/bash
# validate-deployment.sh - Validate pagerduty-operator deployment health
#
# Usage: ./validate-deployment.sh [OPTIONS]
#
# Options:
#   -n, --namespace NAMESPACE    Operator namespace (default: pagerduty-operator)
#   -t, --ticket TICKET          SREP ticket URL for elevation (required)
#   -j, --json                   Output results in JSON format
#   -v, --verbose                Verbose output
#   -h, --help                   Show this help message
#
# Prerequisites:
#   - Must be logged into the hive cluster with oc
#   - Must have ocm backplane access
#   - Must provide SREP ticket for elevation

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
NAMESPACE="pagerduty-operator"
TICKET=""
JSON_OUTPUT=false
VERBOSE=false

# Results tracking
CHECKS_PASSED=0
CHECKS_FAILED=0
CHECKS_WARNING=0
RESULTS=()

# Helper functions
log_info() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${NC}$1${NC}"
    fi
}

log_success() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${GREEN}✅ $1${NC}"
    fi
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
    RESULTS+=("{\"check\":\"$2\",\"status\":\"pass\",\"message\":\"$1\"}")
}

log_fail() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${RED}❌ $1${NC}"
    fi
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
    RESULTS+=("{\"check\":\"$2\",\"status\":\"fail\",\"message\":\"$1\"}")
}

log_warning() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${YELLOW}⚠️  $1${NC}"
    fi
    CHECKS_WARNING=$((CHECKS_WARNING + 1))
    RESULTS+=("{\"check\":\"$2\",\"status\":\"warning\",\"message\":\"$1\"}")
}

verbose() {
    if [[ "$VERBOSE" == "true" && "$JSON_OUTPUT" == "false" ]]; then
        echo "  → $1"
    fi
}

show_help() {
    grep '^#' "$0" | grep -v '#!/bin/bash' | sed 's/^# //' | sed 's/^#//'
    exit 0
}

# Elevated oc command wrapper
elevated_oc() {
    ocm backplane elevate "$TICKET" -- "$@" 2>/dev/null
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -t|--ticket)
            TICKET="$2"
            shift 2
            ;;
        -j|--json)
            JSON_OUTPUT=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            ;;
    esac
done

# Validate required arguments
if [[ -z "$TICKET" ]]; then
    echo "Error: SREP ticket URL is required (-t|--ticket)" >&2
    echo "" >&2
    show_help
fi

# Main validation logic
main() {
    log_info "Starting pagerduty-operator deployment validation"
    log_info "Namespace: $NAMESPACE"
    log_info ""

    # Check 1: Verify we're logged into a cluster
    log_info "[1/5] Verifying cluster connection..."
    if ! oc whoami &>/dev/null; then
        log_fail "Not logged into a cluster. Please login with 'oc login' first." "cluster_connection"
        exit 1
    fi
    CLUSTER=$(oc whoami --show-server)
    verbose "Connected to: $CLUSTER"
    log_success "Connected to cluster" "cluster_connection"

    # Check 2: Verify namespace exists
    log_info "[2/5] Verifying namespace exists..."
    if ! elevated_oc get namespace "$NAMESPACE" &>/dev/null; then
        log_fail "Namespace '$NAMESPACE' does not exist" "namespace_exists"
        exit 1
    fi
    log_success "Namespace '$NAMESPACE' exists" "namespace_exists"

    # Check 3: Check pod status
    log_info "[3/5] Checking pod status..."
    POD_STATUS=$(elevated_oc get pods -n "$NAMESPACE" -l name=pagerduty-operator -o json 2>/dev/null || echo "{}")

    if [[ "$POD_STATUS" == "{}" ]] || [[ $(echo "$POD_STATUS" | jq -r '.items | length') -eq 0 ]]; then
        log_fail "No pagerduty-operator pods found" "pod_exists"
    else
        POD_COUNT=$(echo "$POD_STATUS" | jq -r '.items | length')
        verbose "Found $POD_COUNT pod(s)"

        RUNNING_PODS=0
        for i in $(seq 0 $((POD_COUNT - 1))); do
            POD_NAME=$(echo "$POD_STATUS" | jq -r ".items[$i].metadata.name")
            POD_PHASE=$(echo "$POD_STATUS" | jq -r ".items[$i].status.phase")
            RESTART_COUNT=$(echo "$POD_STATUS" | jq -r ".items[$i].status.containerStatuses[0].restartCount // 0")
            POD_IMAGE=$(echo "$POD_STATUS" | jq -r ".items[$i].status.containerStatuses[0].imageID // \"unknown\"" | sed 's/.*@//')

            verbose "Pod: $POD_NAME | Phase: $POD_PHASE | Restarts: $RESTART_COUNT"
            verbose "Image: $POD_IMAGE"

            if [[ "$POD_PHASE" != "Running" ]]; then
                log_fail "Pod $POD_NAME is not Running (phase: $POD_PHASE)" "pod_running"
            else
                RUNNING_PODS=$((RUNNING_PODS + 1))
                if [[ "$RESTART_COUNT" -gt 0 ]]; then
                    log_warning "Pod $POD_NAME has $RESTART_COUNT restart(s)" "pod_restarts"
                else
                    log_success "Pod $POD_NAME is Running with 0 restarts" "pod_health"
                fi
            fi
        done

        if [[ $RUNNING_PODS -eq $POD_COUNT ]]; then
            log_success "All $POD_COUNT pod(s) are Running" "pods_running"
        fi
    fi

    # Check 4: Check for ERROR logs
    log_info "[4/5] Checking for ERROR-level logs..."
    verbose "Scanning pod logs for errors..."
    ERROR_LOGS=$(elevated_oc logs -n "$NAMESPACE" -l name=pagerduty-operator 2>/dev/null | grep '"level":"error"' || echo "")

    if [[ -z "$ERROR_LOGS" ]]; then
        log_success "No ERROR-level logs found" "error_logs"
    else
        ERROR_COUNT=$(echo "$ERROR_LOGS" | wc -l)

        # Check if errors are all benign (namespace terminating)
        BENIGN_COUNT=$(echo "$ERROR_LOGS" | grep "is being terminated" | wc -l)

        if [[ "$BENIGN_COUNT" -eq "$ERROR_COUNT" ]]; then
            log_warning "Found $ERROR_COUNT ERROR log(s), all benign (namespace terminating)" "error_logs"
        else
            NON_BENIGN=$((ERROR_COUNT - BENIGN_COUNT))
            log_warning "Found $ERROR_COUNT ERROR log(s) ($NON_BENIGN non-benign)" "error_logs"
        fi

        if [[ "$VERBOSE" == "true" ]]; then
            verbose "Most recent errors (last 10):"
            echo "$ERROR_LOGS" | tail -10 | while read -r line; do
                verbose "  $line"
            done
        fi
    fi

    # Check 5: Verify deployment exists and desired replicas
    log_info "[5/5] Checking deployment configuration..."
    DEPLOYMENT=$(elevated_oc get deployment -n "$NAMESPACE" pagerduty-operator -o json 2>/dev/null || echo "{}")

    if [[ "$DEPLOYMENT" == "{}" ]]; then
        log_fail "Deployment 'pagerduty-operator' not found" "deployment_exists"
    else
        DESIRED=$(echo "$DEPLOYMENT" | jq -r '.spec.replicas')
        AVAILABLE=$(echo "$DEPLOYMENT" | jq -r '.status.availableReplicas // 0')
        READY=$(echo "$DEPLOYMENT" | jq -r '.status.readyReplicas // 0')

        verbose "Desired: $DESIRED | Available: $AVAILABLE | Ready: $READY"

        if [[ "$AVAILABLE" -eq "$DESIRED" && "$READY" -eq "$DESIRED" ]]; then
            log_success "Deployment has $DESIRED/$DESIRED replicas available and ready" "deployment_replicas"
        else
            log_fail "Deployment replica mismatch - Desired: $DESIRED, Available: $AVAILABLE, Ready: $READY" "deployment_replicas"
        fi
    fi

    # Summary
    log_info ""
    log_info "========================================="
    log_info "Validation Summary"
    log_info "========================================="

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        echo "{"
        echo "  \"namespace\": \"$NAMESPACE\","
        echo "  \"cluster\": \"$CLUSTER\","
        echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
        echo "  \"summary\": {"
        echo "    \"passed\": $CHECKS_PASSED,"
        echo "    \"failed\": $CHECKS_FAILED,"
        echo "    \"warnings\": $CHECKS_WARNING"
        echo "  },"
        echo "  \"checks\": ["
        echo "    $(IFS=,; echo "${RESULTS[*]}")"
        echo "  ]"
        echo "}"
    else
        log_info "Passed:   $CHECKS_PASSED"
        log_info "Failed:   $CHECKS_FAILED"
        log_info "Warnings: $CHECKS_WARNING"
        log_info ""

        if [[ $CHECKS_FAILED -gt 0 ]]; then
            log_fail "Validation FAILED - $CHECKS_FAILED critical issues found" "overall"
            exit 1
        elif [[ $CHECKS_WARNING -gt 0 ]]; then
            log_warning "Validation PASSED with $CHECKS_WARNING warning(s)" "overall"
            exit 0
        else
            log_success "Validation PASSED - All checks successful" "overall"
            exit 0
        fi
    fi
}

main
