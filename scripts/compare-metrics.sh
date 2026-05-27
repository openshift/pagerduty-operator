#!/bin/bash
# compare-metrics.sh - Compare pagerduty-operator metrics before/after deployment
#
# Usage:
#   ./compare-metrics.sh --baseline -t TICKET > baseline.json
#   ./compare-metrics.sh --compare baseline.json -t TICKET
#
# Options:
#   -n, --namespace NAMESPACE    Operator namespace (default: pagerduty-operator)
#   -t, --ticket TICKET          SREP ticket number for elevation (required)
#   -b, --baseline               Capture baseline metrics snapshot
#   -c, --compare FILE           Compare current metrics against baseline file
#   -v, --verbose                Verbose output
#   -h, --help                   Show this help message
#
# Prerequisites:
#   - Must be logged into the hive cluster with oc
#   - Must have ocm backplane access
#   - Must provide SREP ticket for elevation
#
# Note: This script is READ-ONLY and does not modify any resources

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
NAMESPACE="pagerduty-operator"
TICKET=""
BASELINE_MODE=false
COMPARE_MODE=false
COMPARE_FILE=""
VERBOSE=false

# Prometheus details
PROMETHEUS_NAMESPACE="openshift-customer-monitoring"
PROMETHEUS_POD=""

# Helper functions
log_info() {
    echo -e "${NC}$1${NC}" >&2
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}" >&2
}

log_fail() {
    echo -e "${RED}❌ $1${NC}" >&2
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}" >&2
}

verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo "  → $1" >&2
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
        -b|--baseline)
            BASELINE_MODE=true
            shift
            ;;
        -c|--compare)
            COMPARE_MODE=true
            COMPARE_FILE="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo "Unknown option: $1" >&2
            show_help
            ;;
    esac
done

# Validate required arguments
if [[ -z "$TICKET" ]]; then
    echo "Error: SREP ticket number is required (-t|--ticket)" >&2
    echo "" >&2
    show_help
fi

if [[ "$BASELINE_MODE" == "false" && "$COMPARE_MODE" == "false" ]]; then
    echo "Error: Must specify either --baseline or --compare FILE" >&2
    echo "" >&2
    show_help
fi

if [[ "$BASELINE_MODE" == "true" && "$COMPARE_MODE" == "true" ]]; then
    echo "Error: Cannot specify both --baseline and --compare" >&2
    echo "" >&2
    show_help
fi

if [[ "$COMPARE_MODE" == "true" && ! -f "$COMPARE_FILE" ]]; then
    echo "Error: Baseline file not found: $COMPARE_FILE" >&2
    exit 1
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
    local default="${2:-0}"

    # Check if query was successful
    if [[ $(echo "$response" | jq -r '.status' 2>/dev/null) != "success" ]]; then
        echo "$default"
        return 1
    fi

    # Extract first result value
    local value=$(echo "$response" | jq -r ".data.result[0].value[1] // \"$default\"" 2>/dev/null)
    echo "$value"
}

# Extract histogram quantile
extract_histogram_quantile() {
    local metric_name="$1"
    local quantile="$2"
    local query="histogram_quantile($quantile, rate(${metric_name}_bucket[5m]))"

    local response=$(query_prometheus "$query")
    extract_value "$response" "0"
}

# Capture current metrics snapshot
capture_metrics() {
    log_info "Capturing metrics snapshot..." >&2

    # Find Prometheus pod
    PROMETHEUS_POD=$(oc get pods -n "$PROMETHEUS_NAMESPACE" -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    # If not found, might be named prometheus-app-sre
    if [[ -z "$PROMETHEUS_POD" ]]; then
        PROMETHEUS_POD=$(oc get pods -n "$PROMETHEUS_NAMESPACE" -l prometheus=app-sre -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    fi

    if [[ -z "$PROMETHEUS_POD" ]]; then
        log_fail "Could not find Prometheus pod in $PROMETHEUS_NAMESPACE" >&2
        exit 1
    fi
    verbose "Found Prometheus pod: $PROMETHEUS_POD"

    # Test backplane elevation
    TEST_RESULT=$(ocm backplane elevate "$TICKET" -- exec -n "$PROMETHEUS_NAMESPACE" "$PROMETHEUS_POD" -c prometheus -- echo "success" 2>/dev/null || echo "failed")

    if [[ "$TEST_RESULT" != "success" ]]; then
        log_fail "Backplane elevation failed - check ticket number and permissions" >&2
        exit 1
    fi

    # Capture metrics
    verbose "Querying metrics..."

    SECRET_LOADED=$(extract_value "$(query_prometheus 'pagerdutyintegration_secret_loaded')")
    CREATE_FAILURE=$(extract_value "$(query_prometheus 'pagerduty_create_failure')")
    DELETE_FAILURE=$(extract_value "$(query_prometheus 'pagerduty_delete_failure')")
    HEARTBEAT=$(extract_value "$(query_prometheus 'pagerduty_heartbeat_count')")

    RECONCILE_P50=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.5")
    RECONCILE_P95=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.95")
    RECONCILE_P99=$(extract_histogram_quantile "pagerduty_operator_reconcile_duration_seconds" "0.99")

    API_P50=$(extract_histogram_quantile "pagerduty_operator_api_request_duration_seconds" "0.5")
    API_P95=$(extract_histogram_quantile "pagerduty_operator_api_request_duration_seconds" "0.95")
    API_P99=$(extract_histogram_quantile "pagerduty_operator_api_request_duration_seconds" "0.99")

    # Get pod resource usage
    POD_NAME=$(oc get pods -n "$NAMESPACE" -l name=pagerduty-operator -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    POD_MEMORY="0"
    POD_CPU="0"

    if [[ -n "$POD_NAME" ]]; then
        POD_METRICS=$(oc adm top pod -n "$NAMESPACE" "$POD_NAME" --no-headers 2>/dev/null || echo "")
        if [[ -n "$POD_METRICS" ]]; then
            POD_CPU=$(echo "$POD_METRICS" | awk '{print $2}' | sed 's/m//')
            POD_MEMORY=$(echo "$POD_METRICS" | awk '{print $3}' | sed 's/Mi//')
        fi
    fi

    # Output JSON
    cat <<EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "namespace": "$NAMESPACE",
  "cluster": "$(oc whoami --show-server)",
  "metrics": {
    "secret_loaded": $SECRET_LOADED,
    "create_failure": $CREATE_FAILURE,
    "delete_failure": $DELETE_FAILURE,
    "heartbeat_count": $HEARTBEAT,
    "reconcile_latency": {
      "p50": $RECONCILE_P50,
      "p95": $RECONCILE_P95,
      "p99": $RECONCILE_P99
    },
    "api_latency": {
      "p50": $API_P50,
      "p95": $API_P95,
      "p99": $API_P99
    },
    "resources": {
      "cpu_millicores": $POD_CPU,
      "memory_mib": $POD_MEMORY
    }
  }
}
EOF
}

# Compare current metrics against baseline
compare_metrics() {
    log_info "Comparing metrics against baseline: $COMPARE_FILE" >&2

    # Load baseline
    BASELINE=$(cat "$COMPARE_FILE")
    BASELINE_TIME=$(echo "$BASELINE" | jq -r '.timestamp')

    log_info "Baseline captured: $BASELINE_TIME" >&2
    log_info "" >&2

    # Capture current metrics
    CURRENT=$(capture_metrics)

    # Extract baseline values
    B_SECRET=$(echo "$BASELINE" | jq -r '.metrics.secret_loaded')
    B_CREATE_FAIL=$(echo "$BASELINE" | jq -r '.metrics.create_failure')
    B_DELETE_FAIL=$(echo "$BASELINE" | jq -r '.metrics.delete_failure')
    B_HEARTBEAT=$(echo "$BASELINE" | jq -r '.metrics.heartbeat_count')
    B_RECONCILE_P95=$(echo "$BASELINE" | jq -r '.metrics.reconcile_latency.p95')
    B_RECONCILE_P99=$(echo "$BASELINE" | jq -r '.metrics.reconcile_latency.p99')
    B_API_P95=$(echo "$BASELINE" | jq -r '.metrics.api_latency.p95')
    B_CPU=$(echo "$BASELINE" | jq -r '.metrics.resources.cpu_millicores')
    B_MEMORY=$(echo "$BASELINE" | jq -r '.metrics.resources.memory_mib')

    # Extract current values
    C_SECRET=$(echo "$CURRENT" | jq -r '.metrics.secret_loaded')
    C_CREATE_FAIL=$(echo "$CURRENT" | jq -r '.metrics.create_failure')
    C_DELETE_FAIL=$(echo "$CURRENT" | jq -r '.metrics.delete_failure')
    C_HEARTBEAT=$(echo "$CURRENT" | jq -r '.metrics.heartbeat_count')
    C_RECONCILE_P95=$(echo "$CURRENT" | jq -r '.metrics.reconcile_latency.p95')
    C_RECONCILE_P99=$(echo "$CURRENT" | jq -r '.metrics.reconcile_latency.p99')
    C_API_P95=$(echo "$CURRENT" | jq -r '.metrics.api_latency.p95')
    C_CPU=$(echo "$CURRENT" | jq -r '.metrics.resources.cpu_millicores')
    C_MEMORY=$(echo "$CURRENT" | jq -r '.metrics.resources.memory_mib')

    # Compare and report
    log_info "=========================================" >&2
    log_info "Metrics Comparison" >&2
    log_info "=========================================" >&2

    # Secret loaded
    if [[ "$C_SECRET" == "$B_SECRET" && "$C_SECRET" == "1" ]]; then
        log_success "Secret loaded: OK (both 1)" >&2
    elif [[ "$C_SECRET" != "1" ]]; then
        log_fail "Secret loaded: FAILED (current: $C_SECRET, baseline: $B_SECRET)" >&2
    fi

    # Failures
    if [[ "$C_CREATE_FAIL" == "0" && "$C_DELETE_FAIL" == "0" ]]; then
        log_success "No create/delete failures" >&2
    else
        log_fail "Failures detected - Create: $C_CREATE_FAIL (baseline: $B_CREATE_FAIL), Delete: $C_DELETE_FAIL (baseline: $B_DELETE_FAIL)" >&2
    fi

    # Heartbeat
    if awk -v hb="$C_HEARTBEAT" 'BEGIN { exit !(hb > 0) }'; then
        log_success "Heartbeat active: $C_HEARTBEAT (baseline: $B_HEARTBEAT)" >&2
    else
        log_fail "Heartbeat inactive: $C_HEARTBEAT (baseline: $B_HEARTBEAT)" >&2
    fi

    # Reconcile latency
    RECONCILE_P95_CHANGE=$(awk -v c="$C_RECONCILE_P95" -v b="$B_RECONCILE_P95" 'BEGIN { printf "%.2f", (c - b) / b * 100 }')
    RECONCILE_P99_CHANGE=$(awk -v c="$C_RECONCILE_P99" -v b="$B_RECONCILE_P99" 'BEGIN { printf "%.2f", (c - b) / b * 100 }')

    log_info "" >&2
    log_info "Reconciliation Latency:" >&2
    log_info "  p95: ${C_RECONCILE_P95}s (baseline: ${B_RECONCILE_P95}s, change: ${RECONCILE_P95_CHANGE}%)" >&2
    log_info "  p99: ${C_RECONCILE_P99}s (baseline: ${B_RECONCILE_P99}s, change: ${RECONCILE_P99_CHANGE}%)" >&2

    if awk -v change="$RECONCILE_P95_CHANGE" 'BEGIN { exit !(change > 100) }'; then
        log_fail "Reconcile p95 latency increased >100% (${RECONCILE_P95_CHANGE}%)" >&2
    elif awk -v change="$RECONCILE_P95_CHANGE" 'BEGIN { exit !(change > 50) }'; then
        log_warning "Reconcile p95 latency increased >50% (${RECONCILE_P95_CHANGE}%)" >&2
    else
        log_success "Reconcile latency within acceptable range" >&2
    fi

    # API latency
    API_P95_CHANGE=$(awk -v c="$C_API_P95" -v b="$B_API_P95" 'BEGIN { if (b > 0) printf "%.2f", (c - b) / b * 100; else print "0" }')

    log_info "" >&2
    log_info "API Request Latency:" >&2
    log_info "  p95: ${C_API_P95}s (baseline: ${B_API_P95}s, change: ${API_P95_CHANGE}%)" >&2

    # Memory
    MEMORY_CHANGE=$(awk -v c="$C_MEMORY" -v b="$B_MEMORY" 'BEGIN { if (b > 0) printf "%.2f", (c - b) / b * 100; else print "0" }')

    log_info "" >&2
    log_info "Resource Usage:" >&2
    log_info "  CPU: ${C_CPU}m (baseline: ${B_CPU}m)" >&2
    log_info "  Memory: ${C_MEMORY}Mi (baseline: ${B_MEMORY}Mi, change: ${MEMORY_CHANGE}%)" >&2

    if awk -v change="$MEMORY_CHANGE" 'BEGIN { exit !(change > 50) }'; then
        log_warning "Memory usage increased >50% (${MEMORY_CHANGE}%)" >&2
    else
        log_success "Resource usage within acceptable range" >&2
    fi

    log_info "" >&2
    log_info "=========================================" >&2

    # Return current snapshot to stdout for chaining
    echo "$CURRENT"
}

# Main
main() {
    if [[ "$BASELINE_MODE" == "true" ]]; then
        capture_metrics
    elif [[ "$COMPARE_MODE" == "true" ]]; then
        compare_metrics
    fi
}

main
