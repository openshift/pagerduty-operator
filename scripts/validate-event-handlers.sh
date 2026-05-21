#!/bin/bash
# validate-event-handlers.sh - Validate pagerduty-operator event handler behavior
#
# Usage: ./validate-event-handlers.sh [OPTIONS]
#
# Options:
#   -n, --namespace NAMESPACE    Operator namespace (default: pagerduty-operator)
#   -t, --ticket TICKET          SREP ticket URL for elevation (required)
#   -j, --json                   Output results in JSON format
#   -v, --verbose                Verbose output
#   -h, --help                   Show this help message
#
# Purpose:
#   Validates that the operator's custom event handlers are registered and
#   functioning correctly. Run BEFORE an upgrade to capture a baseline, and
#   AFTER an upgrade to confirm no regressions.
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
BLUE='\033[0;34m'
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

log_info() {
    if [[ "${JSON_OUTPUT}" == "false" ]]; then
        echo -e "${BLUE}${1}${NC}"
    fi
}

log_success() {
    if [[ "${JSON_OUTPUT}" == "false" ]]; then
        echo -e "${GREEN}PASS: ${1}${NC}"
    fi
    CHECKS_PASSED=$((CHECKS_PASSED + 1))
    RESULTS+=("{\"check\":\"${2}\",\"status\":\"pass\",\"message\":\"${1}\"}")
}

log_fail() {
    if [[ "${JSON_OUTPUT}" == "false" ]]; then
        echo -e "${RED}FAIL: ${1}${NC}"
    fi
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
    RESULTS+=("{\"check\":\"${2}\",\"status\":\"fail\",\"message\":\"${1}\"}")
}

log_warning() {
    if [[ "${JSON_OUTPUT}" == "false" ]]; then
        echo -e "${YELLOW}WARN: ${1}${NC}"
    fi
    CHECKS_WARNING=$((CHECKS_WARNING + 1))
    RESULTS+=("{\"check\":\"${2}\",\"status\":\"warning\",\"message\":\"${1}\"}")
}

verbose() {
    if [[ "${VERBOSE}" == "true" && "${JSON_OUTPUT}" == "false" ]]; then
        echo "  -> ${1}"
    fi
}

show_help() {
    grep '^#' "${0}" | grep --invert-match '#!/bin/bash' | sed 's/^# //' | sed 's/^#//'
    exit 0
}

elevated_oc() {
    ocm backplane elevate "${TICKET}" -- "${@}" 2>/dev/null
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case ${1} in
        -n|--namespace)
            NAMESPACE="${2}"
            shift 2
            ;;
        -t|--ticket)
            TICKET="${2}"
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
            echo "Unknown option: ${1}"
            show_help
            ;;
    esac
done

if [[ -z "${TICKET}" ]]; then
    echo "Error: SREP ticket URL is required (-t|--ticket)" >&2
    echo "" >&2
    show_help
fi

print_results() {
    echo ""
    if [[ "${JSON_OUTPUT}" == "true" ]]; then
        echo "{\"passed\":${CHECKS_PASSED},\"failed\":${CHECKS_FAILED},\"warning\":${CHECKS_WARNING},\"results\":[$(IFS=,; echo "${RESULTS[*]}")]}"
    else
        echo "========================================"
        echo "Event Handler Validation Results"
        echo "========================================"
        echo -e "${GREEN}Passed:  ${CHECKS_PASSED}${NC}"
        echo -e "${RED}Failed:  ${CHECKS_FAILED}${NC}"
        echo -e "${YELLOW}Warning: ${CHECKS_WARNING}${NC}"
        echo "========================================"
    fi
}

main() {
    log_info "Starting event handler validation"
    log_info "Namespace: ${NAMESPACE}"
    echo ""

    # Check 1: Cluster connection
    log_info "[1/7] Verifying cluster connection..."
    if ! oc whoami &>/dev/null; then
        log_fail "Not logged into a cluster" "cluster_connection"
        print_results
        exit 1
    fi
    CLUSTER=$(oc whoami --show-server)
    verbose "Connected to: ${CLUSTER}"
    log_success "Connected to cluster" "cluster_connection"

    # Get operator pod name for subsequent checks
    POD_NAME=$(elevated_oc get pods -n "${NAMESPACE}" -l name=pagerduty-operator \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [[ -z "${POD_NAME}" ]]; then
        log_fail "No pagerduty-operator pod found in namespace ${NAMESPACE}" "pod_exists"
        print_results
        exit 1
    fi
    verbose "Operator pod: ${POD_NAME}"

    # Fetch logs once for all checks
    LOGS=$(elevated_oc logs -n "${NAMESPACE}" "${POD_NAME}" --tail=5000 2>/dev/null || echo "")
    if [[ -z "${LOGS}" ]]; then
        log_fail "Could not retrieve operator logs" "log_retrieval"
        print_results
        exit 1
    fi
    verbose "Retrieved $(echo "${LOGS}" | wc -l) log lines"

    # Check 2: Watch registration
    log_info "[2/7] Checking watch registration..."
    WATCH_COUNT=$(echo "${LOGS}" | grep --count "Starting EventSource" 2>/dev/null || echo "0")
    verbose "Found ${WATCH_COUNT} EventSource registrations"
    if [[ "${WATCH_COUNT}" -ge 5 ]]; then
        log_success "All watches registered (${WATCH_COUNT} EventSource entries)" "watch_registration"
    elif [[ "${WATCH_COUNT}" -gt 0 ]]; then
        log_warning "Only ${WATCH_COUNT} EventSource registrations found (expected >= 5)" "watch_registration"
    else
        log_fail "No EventSource registrations found — watches may not be starting" "watch_registration"
    fi

    # Check 3: Reconciliation activity
    log_info "[3/7] Checking reconciliation activity..."
    RECONCILE_COUNT=$(echo "${LOGS}" | grep --count "Reconciling PagerDutyIntegration" 2>/dev/null || echo "0")
    verbose "Found ${RECONCILE_COUNT} reconciliation entries"
    if [[ "${RECONCILE_COUNT}" -gt 0 ]]; then
        log_success "Event handlers firing — ${RECONCILE_COUNT} reconciliation(s) logged" "reconciliation_activity"
    else
        log_warning "No reconciliation entries found — handlers may not be enqueueing requests" "reconciliation_activity"
    fi

    # Check 4: Reconcile completion
    log_info "[4/7] Checking reconcile completions..."
    COMPLETE_COUNT=$(echo "${LOGS}" | grep --count "Reconcile complete" 2>/dev/null || echo "0")
    verbose "Found ${COMPLETE_COUNT} completed reconciliations"
    if [[ "${COMPLETE_COUNT}" -gt 0 ]]; then
        LAST_DURATION=$(echo "${LOGS}" | grep "Reconcile complete" | tail -1 | grep --only-matching '"Duration":"[^"]*"' || echo "unknown")
        verbose "Last reconcile duration: ${LAST_DURATION}"
        log_success "Reconcile completing successfully (${COMPLETE_COUNT} completions)" "reconcile_completion"
    else
        log_warning "No reconcile completions found — reconciler may be erroring" "reconcile_completion"
    fi

    # Check 5: Error-free handler operation
    log_info "[5/7] Checking for handler errors..."
    PANIC_COUNT=$(echo "${LOGS}" | grep --count --ignore-case "panic\|runtime error" 2>/dev/null || echo "0")
    RESTART_COUNT=$(elevated_oc get pod -n "${NAMESPACE}" "${POD_NAME}" \
        -o jsonpath='{.status.containerStatuses[0].restartCount}' 2>/dev/null || echo "unknown")
    verbose "Panics in logs: ${PANIC_COUNT}"
    verbose "Pod restart count: ${RESTART_COUNT}"
    if [[ "${PANIC_COUNT}" -gt 0 ]]; then
        log_fail "Found ${PANIC_COUNT} panic/runtime error entries in logs" "handler_errors"
    elif [[ "${RESTART_COUNT}" != "0" && "${RESTART_COUNT}" != "unknown" ]]; then
        log_warning "Pod has restarted ${RESTART_COUNT} time(s) — may indicate handler crashes" "handler_errors"
    else
        log_success "No panics or restarts detected" "handler_errors"
    fi

    # Check 6: Heartbeat activity
    log_info "[6/7] Checking PD API heartbeat..."
    HEARTBEAT_COUNT=$(echo "${LOGS}" | grep --count "Metrics for PD API" 2>/dev/null || echo "0")
    verbose "Found ${HEARTBEAT_COUNT} heartbeat entries"
    if [[ "${HEARTBEAT_COUNT}" -gt 0 ]]; then
        log_success "PD API heartbeat active (${HEARTBEAT_COUNT} entries)" "heartbeat_activity"
    else
        log_warning "No PD API heartbeat entries found" "heartbeat_activity"
    fi

    # Check 7: Image version
    log_info "[7/7] Checking deployed image version..."
    POD_IMAGE=$(elevated_oc get pod -n "${NAMESPACE}" "${POD_NAME}" \
        -o jsonpath='{.spec.containers[0].image}' 2>/dev/null || echo "unknown")
    POD_IMAGE_TAG=$(echo "${POD_IMAGE}" | sed 's/.*://')
    verbose "Image: ${POD_IMAGE}"
    if [[ "${POD_IMAGE}" != "unknown" ]]; then
        log_success "Operator image: ${POD_IMAGE_TAG}" "image_version"
    else
        log_warning "Could not determine operator image" "image_version"
    fi

    print_results

    if [[ ${CHECKS_FAILED} -gt 0 ]]; then
        exit 1
    fi
}

main
