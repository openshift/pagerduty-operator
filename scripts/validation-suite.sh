#!/bin/bash
# validation-suite.sh - Run complete validation suite for pagerduty-operator
#
# Usage: ./validation-suite.sh [OPTIONS]
#
# Options:
#   -e, --environment ENV        Environment (integration|staging|production)
#   -n, --namespace NAMESPACE    Operator namespace (default: pagerduty-operator)
#   -t, --ticket TICKET          SREP ticket number for elevation (required)
#   -b, --baseline FILE          Optional baseline file for metrics comparison
#   -s, --skip CHECKS            Comma-separated list of checks to skip (deployment,metrics,functional)
#   -j, --json                   Output results in JSON format
#   -v, --verbose                Verbose output
#   -h, --help                   Show this help message
#
# Prerequisites:
#   - Must be logged into the hive cluster with oc
#   - Must have ocm backplane access for metrics validation
#   - Must provide SREP ticket for elevation
#
# Note: This script is READ-ONLY and does not modify any resources

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
ENVIRONMENT=""
NAMESPACE="pagerduty-operator"
TICKET=""
BASELINE_FILE=""
SKIP_CHECKS=""
JSON_OUTPUT=false
VERBOSE=false

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Results tracking
DEPLOYMENT_STATUS="PENDING"
METRICS_STATUS="PENDING"
FUNCTIONAL_STATUS="PENDING"
OVERALL_STATUS="PENDING"

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
}

log_fail() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${RED}❌ $1${NC}"
    fi
}

log_warning() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${YELLOW}⚠️  $1${NC}"
    fi
}

log_header() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${BLUE}=========================================${NC}"
        echo -e "${BLUE}$1${NC}"
        echo -e "${BLUE}=========================================${NC}"
    fi
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

# Check if a validation should be skipped
should_skip() {
    local check="$1"
    if [[ "$SKIP_CHECKS" == *"$check"* ]]; then
        return 0  # True, should skip
    fi
    return 1  # False, should not skip
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--environment)
            ENVIRONMENT="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -t|--ticket)
            TICKET="$2"
            shift 2
            ;;
        -b|--baseline)
            BASELINE_FILE="$2"
            shift 2
            ;;
        -s|--skip)
            SKIP_CHECKS="$2"
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

# Validate environment
if [[ -n "$ENVIRONMENT" ]]; then
    case "$ENVIRONMENT" in
        integration|staging|production)
            ;;
        *)
            echo "Error: Invalid environment. Must be one of: integration, staging, production"
            exit 1
            ;;
    esac
fi

# Build common args
COMMON_ARGS="-n $NAMESPACE"
if [[ "$VERBOSE" == "true" ]]; then
    COMMON_ARGS="$COMMON_ARGS -v"
fi

# Main validation logic
main() {
    START_TIME=$(date +%s)

    if [[ "$JSON_OUTPUT" == "false" ]]; then
        log_header "PagerDuty Operator Validation Suite"
        log_info "Environment:  ${ENVIRONMENT:-Not specified}"
        log_info "Namespace:    $NAMESPACE"
        log_info "SREP Ticket:  $TICKET"
        log_info "Cluster:      $(oc whoami --show-server 2>/dev/null || echo 'Not connected')"
        log_info "Start Time:   $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        log_info ""
    fi

    # Check 1: Deployment Health
    if ! should_skip "deployment"; then
        log_header "1. Deployment Health Check"

        if [[ "$JSON_OUTPUT" == "true" ]]; then
            DEPLOYMENT_RESULT=$("$SCRIPT_DIR/validate-deployment.sh" $COMMON_ARGS -j 2>&1)
            DEPLOYMENT_EXIT=$?
        else
            "$SCRIPT_DIR/validate-deployment.sh" $COMMON_ARGS
            DEPLOYMENT_EXIT=$?
        fi

        if [[ $DEPLOYMENT_EXIT -eq 0 ]]; then
            DEPLOYMENT_STATUS="PASS"
            log_success "Deployment health check: PASSED"
        else
            DEPLOYMENT_STATUS="FAIL"
            log_fail "Deployment health check: FAILED"
        fi
        log_info ""
    else
        DEPLOYMENT_STATUS="SKIPPED"
        verbose "Skipping deployment health check"
    fi

    # Check 2: Metrics Validation
    if ! should_skip "metrics"; then
        log_header "2. Metrics Validation"

        if [[ "$JSON_OUTPUT" == "true" ]]; then
            METRICS_RESULT=$("$SCRIPT_DIR/check-metrics.sh" $COMMON_ARGS -t "$TICKET" -j 2>&1)
            METRICS_EXIT=$?
        else
            "$SCRIPT_DIR/check-metrics.sh" $COMMON_ARGS -t "$TICKET"
            METRICS_EXIT=$?
        fi

        if [[ $METRICS_EXIT -eq 0 ]]; then
            METRICS_STATUS="PASS"
            log_success "Metrics validation: PASSED"
        else
            METRICS_STATUS="FAIL"
            log_fail "Metrics validation: FAILED"
        fi

        # If baseline provided, compare metrics
        if [[ -n "$BASELINE_FILE" && -f "$BASELINE_FILE" ]]; then
            log_info ""
            log_info "Running metrics comparison against baseline..."
            "$SCRIPT_DIR/compare-metrics.sh" $COMMON_ARGS -t "$TICKET" -c "$BASELINE_FILE" > /dev/null
        fi

        log_info ""
    else
        METRICS_STATUS="SKIPPED"
        verbose "Skipping metrics validation"
    fi

    # Check 3: Functional Validation
    if ! should_skip "functional"; then
        log_header "3. Functional Validation"

        if [[ "$JSON_OUTPUT" == "true" ]]; then
            FUNCTIONAL_RESULT=$("$SCRIPT_DIR/validate-functional.sh" $COMMON_ARGS -j 2>&1)
            FUNCTIONAL_EXIT=$?
        else
            "$SCRIPT_DIR/validate-functional.sh" $COMMON_ARGS
            FUNCTIONAL_EXIT=$?
        fi

        if [[ $FUNCTIONAL_EXIT -eq 0 ]]; then
            FUNCTIONAL_STATUS="PASS"
            log_success "Functional validation: PASSED"
        else
            FUNCTIONAL_STATUS="FAIL"
            log_fail "Functional validation: FAILED"
        fi
        log_info ""
    else
        FUNCTIONAL_STATUS="SKIPPED"
        verbose "Skipping functional validation"
    fi

    # Overall status
    if [[ "$DEPLOYMENT_STATUS" == "FAIL" || "$METRICS_STATUS" == "FAIL" || "$FUNCTIONAL_STATUS" == "FAIL" ]]; then
        OVERALL_STATUS="FAIL"
    elif [[ "$DEPLOYMENT_STATUS" == "PASS" || "$METRICS_STATUS" == "PASS" || "$FUNCTIONAL_STATUS" == "PASS" ]]; then
        OVERALL_STATUS="PASS"
    else
        OVERALL_STATUS="SKIPPED"
    fi

    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    # Final summary
    log_header "Validation Summary"

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        # Output JSON results
        cat <<EOF
{
  "environment": "${ENVIRONMENT:-unknown}",
  "namespace": "$NAMESPACE",
  "cluster": "$(oc whoami --show-server 2>/dev/null || echo 'unknown')",
  "ticket": "$TICKET",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "duration_seconds": $DURATION,
  "overall_status": "$OVERALL_STATUS",
  "checks": {
    "deployment": {
      "status": "$DEPLOYMENT_STATUS",
      "details": ${DEPLOYMENT_RESULT:-null}
    },
    "metrics": {
      "status": "$METRICS_STATUS",
      "details": ${METRICS_RESULT:-null}
    },
    "functional": {
      "status": "$FUNCTIONAL_STATUS",
      "details": ${FUNCTIONAL_RESULT:-null}
    }
  }
}
EOF
    else
        log_info "Deployment Health:      $DEPLOYMENT_STATUS"
        log_info "Metrics Validation:     $METRICS_STATUS"
        log_info "Functional Validation:  $FUNCTIONAL_STATUS"
        log_info ""
        log_info "Overall Status:         $OVERALL_STATUS"
        log_info "Duration:               ${DURATION}s"
        log_info ""

        if [[ "$OVERALL_STATUS" == "PASS" ]]; then
            log_success "✅ ALL VALIDATIONS PASSED"
            log_info ""
            log_info "Next steps for $ENVIRONMENT environment:"
            case "$ENVIRONMENT" in
                integration)
                    log_info "  1. Monitor operator for 1-2 hours"
                    log_info "  2. Capture baseline metrics: ./compare-metrics.sh --baseline -t TICKET > baseline-integration.json"
                    log_info "  3. If stable, proceed with promotion to staging"
                    ;;
                staging)
                    log_info "  1. Monitor operator for 24-48 hours"
                    log_info "  2. Test alert grouping and service orchestration"
                    log_info "  3. Test limited-support label handling"
                    log_info "  4. If stable, proceed with promotion to production canary"
                    ;;
                production)
                    log_info "  1. Monitor canary hives (hivep03uw1, hivep04ew2) for 24 hours"
                    log_info "  2. Compare metrics against baseline"
                    log_info "  3. Validate real production traffic"
                    log_info "  4. If stable, proceed with phased rollout to all hives"
                    ;;
                *)
                    log_info "  Review validation results and proceed as appropriate"
                    ;;
            esac
            exit 0
        else
            log_fail "❌ VALIDATION FAILED"
            log_info ""
            log_info "Recommended actions:"
            log_info "  1. Review failed checks above"
            log_info "  2. Check operator logs: oc logs -n $NAMESPACE -l name=pagerduty-operator"
            log_info "  3. Investigate issues before proceeding"
            log_info "  4. Consider rollback if in production"
            exit 1
        fi
    fi
}

main
