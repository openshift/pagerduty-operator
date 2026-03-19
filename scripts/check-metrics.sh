#!/bin/bash
# check-metrics.sh - Validate pagerduty-operator Prometheus metrics
#
# Usage: ./check-metrics.sh [OPTIONS]
#
# Options:
#   -n, --namespace NAMESPACE    Operator namespace (default: pagerduty-operator)
#   -t, --ticket TICKET          SREP ticket number for elevation (required)
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

# Prometheus details
PROMETHEUS_NAMESPACE="openshift-customer-monitoring"
PROMETHEUS_POD=""

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
    RESULTS+=("{\"check\":\"$2\",\"status\":\"pass\",\"message\":\"$1\",\"value\":\"$3\"}")
}

log_fail() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${RED}❌ $1${NC}"
    fi
    CHECKS_FAILED=$((CHECKS_FAILED + 1))
    RESULTS+=("{\"check\":\"$2\",\"status\":\"fail\",\"message\":\"$1\",\"value\":\"$3\"}")
}

log_warning() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${YELLOW}⚠️  $1${NC}"
    fi
    CHECKS_WARNING=$((CHECKS_WARNING + 1))
    RESULTS+=("{\"check\":\"$2\",\"status\":\"warning\",\"message\":\"$1\",\"value\":\"$3\"}")
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
    echo "Error: SREP ticket number is required (-t|--ticket)"
    echo ""
    show_help
fi

# Query Prometheus via backplane elevation
query_prometheus() {
    local query="$1"
    local result=""

    verbose "Querying Prometheus: $query"

    # Use ocm backplane elevate to execute query inside prometheus pod
    result=$(ocm backplane elevate "$TICKET" -- exec -n "$PROMETHEUS_NAMESPACE" "$PROMETHEUS_POD" -c prometheus -- \
        sh -c "wget -q -O- 'http://localhost:9090/api/v1/query?query=$(echo "$query" | jq -sRr @uri)'" 2>/dev/null || echo '{"status":"error"}')

    echo "$result"
}

# Extract metric value from Prometheus response
extract_value() {
    local response="$1"
    local value=""

    # Check if query was successful
    if [[ $(echo "$response" | jq -r '.status' 2>/dev/null) != "success" ]]; then
        echo "ERROR"
        return 1
    fi

    # Extract first result value
    value=$(echo "$response" | jq -r '.data.result[0].value[1] // "NODATA"' 2>/dev/null)
    echo "$value"
}

# Extract histogram quantile
extract_histogram_quantile() {
    local metric_name="$1"
    local quantile="$2"
    local query="histogram_quantile($quantile, rate(${metric_name}_bucket[5m]))"

    local response=$(query_prometheus "$query")
    extract_value "$response"
}

# Main validation logic
main() {
    log_info "Starting pagerduty-operator metrics validation"
    log_info "Namespace: $NAMESPACE"
    log_info "SREP Ticket: $TICKET"
    log_info ""

    # Check 1: Verify cluster connection
    log_info "[1/8] Verifying cluster connection..."
    if ! oc whoami &>/dev/null; then
        log_fail "Not logged into a cluster" "cluster_connection" "N/A"
        exit 1
    fi
    log_success "Connected to cluster" "cluster_connection" "$(oc whoami --show-server)"

    # Check 2: Find Prometheus pod
    log_info "[2/8] Finding Prometheus pod..."
    PROMETHEUS_POD=$(oc get pods -n "$PROMETHEUS_NAMESPACE" -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    # If not found, might be named prometheus-app-sre
    if [[ -z "$PROMETHEUS_POD" ]]; then
        PROMETHEUS_POD=$(oc get pods -n "$PROMETHEUS_NAMESPACE" -l prometheus=app-sre -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    fi

    if [[ -z "$PROMETHEUS_POD" ]]; then
        log_fail "Could not find Prometheus pod in $PROMETHEUS_NAMESPACE" "prometheus_pod" "N/A"
        exit 1
    fi
    verbose "Found Prometheus pod: $PROMETHEUS_POD"
    log_success "Found Prometheus pod" "prometheus_pod" "$PROMETHEUS_POD"

    # Check 3: Test backplane elevation
    log_info "[3/8] Testing backplane elevation..."
    TEST_RESULT=$(ocm backplane elevate "$TICKET" -- exec -n "$PROMETHEUS_NAMESPACE" "$PROMETHEUS_POD" -c prometheus -- echo "success" 2>/dev/null || echo "failed")

    if [[ "$TEST_RESULT" != "success" ]]; then
        log_fail "Backplane elevation failed - check ticket number and permissions" "backplane_elevation" "failed"
        exit 1
    fi
    log_success "Backplane elevation successful" "backplane_elevation" "success"

    # Check 4: pagerdutyintegration_secret_loaded metric
    log_info "[4/8] Checking pagerdutyintegration_secret_loaded metric..."
    SECRET_LOADED_RESPONSE=$(query_prometheus "pagerdutyintegration_secret_loaded")
    SECRET_LOADED=$(extract_value "$SECRET_LOADED_RESPONSE")

    if [[ "$SECRET_LOADED" == "ERROR" ]] || [[ "$SECRET_LOADED" == "NODATA" ]]; then
        log_fail "pagerdutyintegration_secret_loaded metric not available" "secret_loaded" "$SECRET_LOADED"
    elif [[ "$SECRET_LOADED" == "1" ]]; then
        log_success "API secret loaded successfully" "secret_loaded" "1"
    else
        log_fail "API secret not loaded (value: $SECRET_LOADED)" "secret_loaded" "$SECRET_LOADED"
    fi

    # Check 5: pagerduty_create_failure metric
    log_info "[5/8] Checking pagerduty_create_failure metric..."
    CREATE_FAILURE_RESPONSE=$(query_prometheus "pagerduty_create_failure")
    CREATE_FAILURE=$(extract_value "$CREATE_FAILURE_RESPONSE")

    if [[ "$CREATE_FAILURE" == "ERROR" ]] || [[ "$CREATE_FAILURE" == "NODATA" ]]; then
        log_warning "pagerduty_create_failure metric not available" "create_failure" "$CREATE_FAILURE"
    elif [[ "$CREATE_FAILURE" == "0" ]]; then
        log_success "No PagerDuty creation failures" "create_failure" "0"
    else
        log_fail "PagerDuty creation failures detected (count: $CREATE_FAILURE)" "create_failure" "$CREATE_FAILURE"
    fi

    # Check 6: pagerduty_delete_failure metric
    log_info "[6/8] Checking pagerduty_delete_failure metric..."
    DELETE_FAILURE_RESPONSE=$(query_prometheus "pagerduty_delete_failure")
    DELETE_FAILURE=$(extract_value "$DELETE_FAILURE_RESPONSE")

    if [[ "$DELETE_FAILURE" == "ERROR" ]] || [[ "$DELETE_FAILURE" == "NODATA" ]]; then
        log_warning "pagerduty_delete_failure metric not available" "delete_failure" "$DELETE_FAILURE"
    elif [[ "$DELETE_FAILURE" == "0" ]]; then
        log_success "No PagerDuty deletion failures" "delete_failure" "0"
    else
        log_fail "PagerDuty deletion failures detected (count: $DELETE_FAILURE)" "delete_failure" "$DELETE_FAILURE"
    fi

    # Check 7: pagerduty_heartbeat_count metric
    log_info "[7/8] Checking pagerduty_heartbeat_count metric..."
    HEARTBEAT_RESPONSE=$(query_prometheus "pagerduty_heartbeat_count")
    HEARTBEAT=$(extract_value "$HEARTBEAT_RESPONSE")

    if [[ "$HEARTBEAT" == "ERROR" ]] || [[ "$HEARTBEAT" == "NODATA" ]]; then
        log_fail "pagerduty_heartbeat_count metric not available" "heartbeat" "$HEARTBEAT"
    elif awk -v hb="$HEARTBEAT" 'BEGIN { exit !(hb > 0) }'; then
        log_success "PagerDuty API heartbeat active (count: $HEARTBEAT)" "heartbeat" "$HEARTBEAT"
    else
        log_fail "PagerDuty API heartbeat not active (count: $HEARTBEAT)" "heartbeat" "$HEARTBEAT"
    fi

    # Check 8: Reconciliation latency
    log_info "[8/8] Checking reconciliation latency..."
    P50=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.5")
    P95=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.95")
    P99=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.99")

    verbose "p50: ${P50}s | p95: ${P95}s | p99: ${P99}s"

    if [[ "$P95" == "ERROR" ]] || [[ "$P95" == "NODATA" ]]; then
        log_warning "Reconciliation latency metrics not available" "reconciliation_latency" "N/A"
    else
        # Check if p95 < 2s and p99 < 5s
        if awk -v p95="$P95" -v p99="$P99" 'BEGIN { exit !(p95 < 2 && p99 < 5) }'; then
            log_success "Reconciliation latency within expected range (p95: ${P95}s, p99: ${P99}s)" "reconciliation_latency" "p95:${P95},p99:${P99}"
        elif awk -v p95="$P95" -v p99="$P99" 'BEGIN { exit !(p95 >= 2 || p99 >= 5) }'; then
            log_warning "Reconciliation latency elevated (p95: ${P95}s, p99: ${P99}s)" "reconciliation_latency" "p95:${P95},p99:${P99}"
        else
            log_success "Reconciliation latency (p50: ${P50}s, p95: ${P95}s, p99: ${P99}s)" "reconciliation_latency" "p50:${P50},p95:${P95},p99:${P99}"
        fi
    fi

    # Summary
    log_info ""
    log_info "========================================="
    log_info "Metrics Validation Summary"
    log_info "========================================="

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        # Output JSON results
        echo "{"
        echo "  \"namespace\": \"$NAMESPACE\","
        echo "  \"ticket\": \"$TICKET\","
        echo "  \"prometheus_pod\": \"$PROMETHEUS_POD\","
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
            log_fail "Metrics validation FAILED - $CHECKS_FAILED critical issues found" "overall" "N/A"
            exit 1
        elif [[ $CHECKS_WARNING -gt 0 ]]; then
            log_warning "Metrics validation PASSED with $CHECKS_WARNING warning(s)" "overall" "N/A"
            exit 0
        else
            log_success "Metrics validation PASSED - All checks successful" "overall" "N/A"
            exit 0
        fi
    fi
}

main
