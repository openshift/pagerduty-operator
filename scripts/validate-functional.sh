#!/bin/bash
# validate-functional.sh - Validate pagerduty-operator functional behavior
#
# Focuses on clusters installed AFTER the operator pod started, to prove
# the current version is actively creating PagerDuty resources correctly.
#
# Validates:
#   - PagerDutyIntegration CRs exist and have finalizers
#   - Operator pod start time (rollout timestamp)
#   - ClusterDeployments installed after rollout have PD finalizers
#   - Those clusters have ConfigMaps (*-pd-config) with SERVICE_ID/INTEGRATION_ID
#   - Those clusters have Secrets (*-pd-secret) (existence only, content NOT read)
#   - Those clusters have SyncSets (*-pd-secret) with valid structure
#   - Logs show successful reconciliation and heartbeat activity
#   - If no new clusters since rollout, reports clearly
#
# All resource checks are scoped to specific CD namespaces, not cluster-wide.
#
# Usage: ./validate-functional.sh [OPTIONS]
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

log_action() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        echo -e "${BLUE}🔵 $1${NC}"
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
    log_info "Starting pagerduty-operator functional validation"
    log_info "Namespace: $NAMESPACE"
    log_info ""

    # ------------------------------------------------------------------
    # Check 1: Verify cluster connection
    # ------------------------------------------------------------------
    log_info "[1/7] Verifying cluster connection..."
    if ! oc whoami &>/dev/null; then
        log_fail "Not logged into a cluster" "cluster_connection" "N/A"
        exit 1
    fi
    log_success "Connected to cluster" "cluster_connection" "$(oc whoami --show-server)"

    # ------------------------------------------------------------------
    # Check 2: PagerDutyIntegration CRs
    # ------------------------------------------------------------------
    log_info "[2/7] Checking PagerDutyIntegration resources..."
    PDI_JSON=$(elevated_oc get pagerdutyintegration -n "$NAMESPACE" -o json 2>/dev/null || echo '{"items":[]}')
    PDI_COUNT=$(echo "$PDI_JSON" | jq -r '.items | length')

    if [[ "$PDI_COUNT" -eq 0 ]]; then
        log_fail "No PagerDutyIntegration resources found" "pdi_exists" "0"
    else
        # Collect service prefixes for resource lookups
        PDI_PREFIXES=()
        for i in $(seq 0 $((PDI_COUNT - 1))); do
            PDI_NAME=$(echo "$PDI_JSON" | jq -r ".items[$i].metadata.name")
            PDI_FINALIZERS=$(echo "$PDI_JSON" | jq -r ".items[$i].metadata.finalizers // [] | length")
            PDI_PREFIX=$(echo "$PDI_JSON" | jq -r ".items[$i].spec.servicePrefix")
            PDI_SELECTOR=$(echo "$PDI_JSON" | jq -c ".items[$i].spec.clusterDeploymentSelector")
            PDI_PREFIXES+=("$PDI_PREFIX")

            verbose "PDI: $PDI_NAME | prefix: $PDI_PREFIX | selector: $PDI_SELECTOR | finalizers: $PDI_FINALIZERS"

            if [[ "$PDI_FINALIZERS" -gt 0 ]]; then
                log_success "PDI '$PDI_NAME' has finalizer (active, prefix: $PDI_PREFIX)" "pdi_finalizer_$PDI_NAME" "$PDI_FINALIZERS"
            else
                log_warning "PDI '$PDI_NAME' has no finalizers" "pdi_finalizer_$PDI_NAME" "0"
            fi
        done
    fi

    # ------------------------------------------------------------------
    # Check 3: Determine operator rollout time
    # ------------------------------------------------------------------
    log_info "[3/7] Determining operator rollout time..."
    POD_JSON=$(elevated_oc get pods -n "$NAMESPACE" -l name=pagerduty-operator -o json 2>/dev/null || echo '{"items":[]}')
    POD_COUNT=$(echo "$POD_JSON" | jq -r '.items | length')

    if [[ "$POD_COUNT" -eq 0 ]]; then
        log_fail "No operator pods found" "operator_pod" "0"
        exit 1
    fi

    # Use the oldest running pod's start time as rollout timestamp
    POD_START=$(echo "$POD_JSON" | jq -r '[.items[] | select(.status.phase == "Running") | .status.startTime] | sort | first // "unknown"')
    POD_NAME=$(echo "$POD_JSON" | jq -r '.items[0].metadata.name')
    POD_IMAGE=$(echo "$POD_JSON" | jq -r '.items[0].status.containerStatuses[0].image // "unknown"' | sed 's|.*/||')

    if [[ "$POD_START" == "unknown" || "$POD_START" == "null" ]]; then
        log_fail "Could not determine operator pod start time" "pod_start_time" "unknown"
        exit 1
    fi

    POD_START_EPOCH=$(date -d "$POD_START" +%s 2>/dev/null || echo "0")
    POD_AGE_HOURS=$(( ($(date +%s) - POD_START_EPOCH) / 3600 ))
    POD_AGE_MINS=$(( (($(date +%s) - POD_START_EPOCH) % 3600) / 60 ))

    log_success "Operator pod started: $POD_START (${POD_AGE_HOURS}h${POD_AGE_MINS}m ago, image: $POD_IMAGE)" "pod_start_time" "$POD_START"

    # ------------------------------------------------------------------
    # Check 4: Find ClusterDeployments installed AFTER operator rollout
    # ------------------------------------------------------------------
    log_info "[4/7] Finding ClusterDeployments installed after operator rollout..."

    # Query ClusterDeployments directly — they are the source of truth
    # for which namespaces contain PD resources
    CD_JSON=$(elevated_oc get clusterdeployment.hive.openshift.io --all-namespaces -o json 2>/dev/null || echo '{"items":[]}')
    CD_TOTAL=$(echo "$CD_JSON" | jq -r '.items | length')
    CD_INSTALLED=$(echo "$CD_JSON" | jq -r '[.items[] | select(.spec.installed == true)] | length')

    # Collect unique namespaces where CDs live (for scoped resource lookups later)
    CD_NAMESPACES=$(echo "$CD_JSON" | jq -r '[.items[].metadata.namespace] | unique | .[]')

    # Filter CDs installed after pod start time
    NEW_CDS=$(echo "$CD_JSON" | jq -r --arg since "$POD_START" \
        '[.items[] | select(.spec.installed == true) |
          select(
            (.status.installedTimestamp // .metadata.creationTimestamp) > $since
          )]')
    NEW_CD_COUNT=$(echo "$NEW_CDS" | jq -r 'length')

    verbose "Total CDs: $CD_TOTAL | Installed: $CD_INSTALLED | Installed after rollout: $NEW_CD_COUNT"

    if [[ "$CD_TOTAL" -eq 0 ]]; then
        log_action "No ClusterDeployments found on this hive — no PD resources expected"
    fi

    if [[ "$NEW_CD_COUNT" -eq 0 ]]; then
        log_action "NO new clusters installed since operator rollout ($POD_START)"
        log_action "Cannot validate new PD service creation with current version"
        log_action "Options:"
        log_action "  1. Wait for a cluster to be provisioned on this hive"
        log_action "  2. Request a test cluster creation"
        log_action "  3. Re-run this script later"
        log_warning "No new clusters to validate since rollout" "new_clusters" "0"
    else
        log_success "Found $NEW_CD_COUNT cluster(s) installed after operator rollout" "new_clusters" "$NEW_CD_COUNT"
    fi

    # ------------------------------------------------------------------
    # Check 5: Spot-check PD resources for new clusters
    # ------------------------------------------------------------------
    if [[ "$NEW_CD_COUNT" -gt 0 ]]; then
        # Filter to eligible CDs before spot-checking (exclude fake/nightly/noalerts)
        ELIGIBLE_FOR_SPOT=$(echo "$NEW_CDS" | jq -r '[.[] |
            select((.metadata.annotations["managed.openshift.com/fake"] // "false") != "true") |
            select((.metadata.labels["api.openshift.com/channel-group"] // "") != "nightly") |
            select((.metadata.labels["api.openshift.com/noalerts"] // "") != "true") |
            select((.metadata.labels["ext-managed.openshift.io/noalerts"] // "") != "true")]
            | sort_by(.status.installedTimestamp // .metadata.creationTimestamp)')
        ELIGIBLE_FOR_SPOT_COUNT=$(echo "$ELIGIBLE_FOR_SPOT" | jq -r 'length')

        if [[ "$ELIGIBLE_FOR_SPOT_COUNT" -eq 0 ]]; then
            log_info "[5/7] Skipping PD resource validation (all $NEW_CD_COUNT new clusters are fake/nightly/noalerts)"
            SPOT_COUNT=0
        else
            if [[ "$ELIGIBLE_FOR_SPOT_COUNT" -le 2 ]]; then
                SPOT_CHECK_CDS="$ELIGIBLE_FOR_SPOT"
                SPOT_COUNT="$ELIGIBLE_FOR_SPOT_COUNT"
            else
                FIRST=$(echo "$ELIGIBLE_FOR_SPOT" | jq '.[0]')
                MIDDLE=$(echo "$ELIGIBLE_FOR_SPOT" | jq ".[$(( ELIGIBLE_FOR_SPOT_COUNT / 2 ))]")
                LAST=$(echo "$ELIGIBLE_FOR_SPOT" | jq '.[-1]')
                SPOT_CHECK_CDS=$(echo "[$FIRST,$MIDDLE,$LAST]" | jq -r '.')
                SPOT_COUNT=3
            fi

            log_info "[5/7] Spot-checking PD resources for $SPOT_COUNT of $ELIGIBLE_FOR_SPOT_COUNT eligible new cluster(s)..."
            log_info "       (Note: secret existence is checked but content is NOT read or decoded)"
        fi

        VALIDATED=0
        VALIDATION_FAILURES=0

        for i in $(seq 0 $((SPOT_COUNT - 1))); do
            CD_NAME=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].metadata.name")
            CD_NS=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].metadata.namespace")
            CD_INSTALLED_TIME=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].status.installedTimestamp // .[$i].metadata.creationTimestamp")

            verbose "Validating cluster: $CD_NS/$CD_NAME (installed: $CD_INSTALLED_TIME)"

            # Check PD finalizer on this CD (from already-fetched data)
            HAS_PD_FINALIZER=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].metadata.finalizers // [] | map(select(startswith(\"pd.managed.openshift.io/\"))) | length")

            # Bulk fetch all ConfigMaps, Secrets, SyncSets from this namespace in one call
            NS_RESOURCES=$(elevated_oc get configmap,secret,syncset -n "$CD_NS" -o json 2>/dev/null || echo '{"items":[]}')

            # Validate PD resources locally
            CM_FOUND=false
            SECRET_FOUND=false
            SS_FOUND=false
            CM_VALID=false
            SS_VALID=false

            for PREFIX in "${PDI_PREFIXES[@]}"; do
                CM_NAME_CHECK="${PREFIX}-${CD_NAME}-pd-config"
                SECRET_NAME_CHECK="${PREFIX}-${CD_NAME}-pd-secret"

                # ConfigMap - find by name and kind in bulk data
                CM_DATA=$(echo "$NS_RESOURCES" | jq -r --arg name "$CM_NAME_CHECK" '.items[] | select(.kind == "ConfigMap" and .metadata.name == $name) // empty')
                if [[ -z "$CM_DATA" ]]; then
                    continue
                fi

                CM_FOUND=true
                verbose "  Matched PDI prefix: $PREFIX"
                SVC_ID=$(echo "$CM_DATA" | jq -r '.data.SERVICE_ID // ""')
                INT_ID=$(echo "$CM_DATA" | jq -r '.data.INTEGRATION_ID // ""')
                ESC_ID=$(echo "$CM_DATA" | jq -r '.data.ESCALATION_POLICY_ID // ""')
                if [[ -n "$SVC_ID" && -n "$INT_ID" && -n "$ESC_ID" ]]; then
                    CM_VALID=true
                    verbose "  ConfigMap $CM_NAME_CHECK: SERVICE_ID=$SVC_ID INTEGRATION_ID=$INT_ID ESCALATION_POLICY_ID=$ESC_ID"
                else
                    verbose "  ConfigMap $CM_NAME_CHECK: MISSING keys (SERVICE_ID='$SVC_ID' INTEGRATION_ID='$INT_ID' ESCALATION_POLICY_ID='$ESC_ID')"
                fi

                # Secret - check existence only (NOT reading content)
                verbose "  Secret $SECRET_NAME_CHECK: checking existence only (not reading content)"
                SECRET_EXISTS=$(echo "$NS_RESOURCES" | jq -r --arg name "$SECRET_NAME_CHECK" '.items[] | select(.kind == "Secret" and .metadata.name == $name) | .metadata.name // empty')
                if [[ -n "$SECRET_EXISTS" ]]; then
                    SECRET_FOUND=true
                    verbose "  Secret $SECRET_NAME_CHECK: exists"
                fi

                # SyncSet - validate structure
                SS_DATA=$(echo "$NS_RESOURCES" | jq -r --arg name "$SECRET_NAME_CHECK" '.items[] | select(.kind == "SyncSet" and .metadata.name == $name) // empty')
                if [[ -n "$SS_DATA" ]]; then
                    SS_FOUND=true
                    SS_CD_REF=$(echo "$SS_DATA" | jq -r '.spec.clusterDeploymentRefs[0].name // ""')
                    SS_APPLY_MODE=$(echo "$SS_DATA" | jq -r '.spec.resourceApplyMode // ""')
                    SS_MAPPING_COUNT=$(echo "$SS_DATA" | jq -r '.spec.secretMappings | length // 0')
                    SS_SOURCE_NAME=$(echo "$SS_DATA" | jq -r '.spec.secretMappings[0].sourceRef.name // ""')
                    SS_TARGET_NAME=$(echo "$SS_DATA" | jq -r '.spec.secretMappings[0].targetRef.name // ""')
                    SS_TARGET_NS=$(echo "$SS_DATA" | jq -r '.spec.secretMappings[0].targetRef.namespace // ""')

                    if [[ "$SS_CD_REF" == "$CD_NAME" && "$SS_APPLY_MODE" == "Sync" && "$SS_MAPPING_COUNT" -gt 0 && -n "$SS_TARGET_NAME" && -n "$SS_TARGET_NS" ]]; then
                        SS_VALID=true
                        verbose "  SyncSet $SECRET_NAME_CHECK: valid (CD ref=$SS_CD_REF, mode=$SS_APPLY_MODE, source=$SS_SOURCE_NAME, target=$SS_TARGET_NS/$SS_TARGET_NAME)"
                    else
                        verbose "  SyncSet $SECRET_NAME_CHECK: INVALID (CD ref='$SS_CD_REF' expected '$CD_NAME', mode='$SS_APPLY_MODE', mappings=$SS_MAPPING_COUNT, target='$SS_TARGET_NS/$SS_TARGET_NAME')"
                    fi
                fi

                break
            done

            # Report results for this cluster
            if [[ "$HAS_PD_FINALIZER" -gt 0 ]]; then
                verbose "  PD finalizer: present"
            else
                verbose "  PD finalizer: MISSING"
            fi

            if $CM_FOUND && $CM_VALID && $SECRET_FOUND && $SS_FOUND && $SS_VALID && [[ "$HAS_PD_FINALIZER" -gt 0 ]]; then
                log_success "Cluster $CD_NS/$CD_NAME: all PD resources present and valid" "new_cd_$CD_NAME" "complete"
                VALIDATED=$((VALIDATED + 1))
            else
                MISSING=""
                [[ "$HAS_PD_FINALIZER" -eq 0 ]] && MISSING="${MISSING}finalizer,"
                $CM_FOUND || MISSING="${MISSING}configmap,"
                $CM_VALID || MISSING="${MISSING}configmap-data,"
                $SECRET_FOUND || MISSING="${MISSING}secret,"
                $SS_FOUND || MISSING="${MISSING}syncset,"
                $SS_VALID || MISSING="${MISSING}syncset-data,"
                MISSING="${MISSING%,}"

                log_fail "Cluster $CD_NS/$CD_NAME: missing [$MISSING]" "new_cd_$CD_NAME" "$MISSING"
                VALIDATION_FAILURES=$((VALIDATION_FAILURES + 1))
            fi
        done

        if [[ "$VALIDATION_FAILURES" -eq 0 && "$VALIDATED" -gt 0 ]]; then
            log_success "All $VALIDATED spot-checked cluster(s) have complete PD resources" "new_cluster_validation" "$VALIDATED/$VALIDATED"
        elif [[ "$VALIDATED" -gt 0 ]]; then
            log_warning "$VALIDATED/$((VALIDATED + VALIDATION_FAILURES)) spot-checked cluster(s) have complete PD resources" "new_cluster_validation" "$VALIDATED/$((VALIDATED + VALIDATION_FAILURES))"
        fi
    else
        log_info "[5/7] Skipping PD resource validation (no new clusters)"
    fi

    # ------------------------------------------------------------------
    # Check 6: PD finalizer counts across all new clusters (from cached data)
    # ------------------------------------------------------------------
    log_info "[6/7] Checking PD finalizers across all new cluster(s)..."

    if [[ "$NEW_CD_COUNT" -gt 0 ]]; then
        # Filter out clusters that are excluded from PD by design (no API calls needed)
        # - Fake clusters (managed.openshift.com/fake annotation)
        # - Nightly channel clusters (channel-group: nightly)
        # - Clusters with noalerts labels
        # - FedRAMP clusters (handled by separate PDI, but excluded from standard ones)
        ELIGIBLE_CDS=$(echo "$NEW_CDS" | jq -r '[.[] |
            select((.metadata.annotations["managed.openshift.com/fake"] // "false") != "true") |
            select((.metadata.labels["api.openshift.com/channel-group"] // "") != "nightly") |
            select((.metadata.labels["api.openshift.com/noalerts"] // "") != "true") |
            select((.metadata.labels["ext-managed.openshift.io/noalerts"] // "") != "true")]')
        ELIGIBLE_COUNT=$(echo "$ELIGIBLE_CDS" | jq -r 'length')
        FILTERED_COUNT=$((NEW_CD_COUNT - ELIGIBLE_COUNT))

        if [[ "$FILTERED_COUNT" -gt 0 ]]; then
            log_info "       (filtering $FILTERED_COUNT cluster(s): fake/nightly/noalerts)"
        fi

        ELIGIBLE_WITH_FINALIZER=$(echo "$ELIGIBLE_CDS" | jq -r '[.[] | select(.metadata.finalizers != null) | select(.metadata.finalizers[] | startswith("pd.managed.openshift.io/"))] | length')
        ELIGIBLE_WITHOUT_FINALIZER=$((ELIGIBLE_COUNT - ELIGIBLE_WITH_FINALIZER))

        if [[ "$ELIGIBLE_COUNT" -eq 0 ]]; then
            verbose "No eligible clusters after filtering (all $NEW_CD_COUNT are fake/nightly/noalerts)"
        elif [[ "$ELIGIBLE_WITHOUT_FINALIZER" -eq 0 ]]; then
            log_success "All $ELIGIBLE_COUNT eligible new cluster(s) have PD finalizers" "new_cd_finalizers" "$ELIGIBLE_WITH_FINALIZER/$ELIGIBLE_COUNT"
        else
            log_warning "$ELIGIBLE_WITHOUT_FINALIZER of $ELIGIBLE_COUNT eligible new cluster(s) missing PD finalizers" "new_cd_finalizers" "$ELIGIBLE_WITH_FINALIZER/$ELIGIBLE_COUNT"
            # List the clusters missing finalizers for SRE investigation
            MISSING_FINALIZER_CDS=$(echo "$ELIGIBLE_CDS" | jq -r '[.[] |
                select(
                    (.metadata.finalizers == null) or
                    ([.metadata.finalizers[] | select(startswith("pd.managed.openshift.io/"))] | length == 0)
                )]')
            MISSING_COUNT=$(echo "$MISSING_FINALIZER_CDS" | jq -r 'length')
            for j in $(seq 0 $((MISSING_COUNT - 1))); do
                MISS_NS=$(echo "$MISSING_FINALIZER_CDS" | jq -r ".[$j].metadata.namespace")
                MISS_NAME=$(echo "$MISSING_FINALIZER_CDS" | jq -r ".[$j].metadata.name")
                MISS_DEL=$(echo "$MISSING_FINALIZER_CDS" | jq -r ".[$j].metadata.deletionTimestamp // empty")
                if [[ -n "$MISS_DEL" ]]; then
                    log_info "       - $MISS_NS/$MISS_NAME (deleting: $MISS_DEL)"
                else
                    log_info "       - $MISS_NS/$MISS_NAME (not deleting)"
                fi
            done
        fi
    else
        verbose "No new clusters to check finalizers for"
    fi

    # ------------------------------------------------------------------
    # Check 7: Log activity (single fetch, heartbeat + errors + creation evidence)
    # ------------------------------------------------------------------
    log_info "[7/7] Checking log activity..."
    RECENT_LOGS=$(elevated_oc logs -n "$NAMESPACE" -l name=pagerduty-operator --tail=200 2>/dev/null || echo "")

    if [[ -z "$RECENT_LOGS" ]]; then
        log_warning "Could not retrieve operator logs" "recent_activity" "no logs"
    else
        # Heartbeat
        HEARTBEAT_LINES=$(echo "$RECENT_LOGS" | grep "Metrics for PD API" | wc -l)
        if [[ "$HEARTBEAT_LINES" -gt 0 ]]; then
            LAST_HEARTBEAT=$(echo "$RECENT_LOGS" | grep "Metrics for PD API" | tail -1 | jq -r '.ts' 2>/dev/null || echo "unknown")
            log_success "PD API heartbeat active (last: $LAST_HEARTBEAT)" "heartbeat_logs" "$HEARTBEAT_LINES"
        else
            log_fail "No PD API heartbeat found in recent logs" "heartbeat_logs" "0"
        fi

        # Successful reconciliations
        RECONCILE_LINES=$(echo "$RECENT_LOGS" | grep "Successfully reconciled" | wc -l)
        if [[ "$RECONCILE_LINES" -gt 0 ]]; then
            LAST_RECONCILE=$(echo "$RECENT_LOGS" | grep "Successfully reconciled" | tail -1 | jq -r '.ts' 2>/dev/null || echo "unknown")
            log_success "Recent successful reconciliations ($RECONCILE_LINES, last: $LAST_RECONCILE)" "reconcile_activity" "$RECONCILE_LINES"
        else
            verbose "No 'Successfully reconciled' messages in recent logs (may be normal if no changes)"
        fi

        # Errors
        ERROR_LINES=$(echo "$RECENT_LOGS" | grep '"level":"error"' | wc -l)
        if [[ "$ERROR_LINES" -gt 0 ]]; then
            BENIGN_ERRORS=$(echo "$RECENT_LOGS" | grep '"level":"error"' | grep "is being terminated" | wc -l)
            NON_BENIGN=$((ERROR_LINES - BENIGN_ERRORS))
            if [[ "$NON_BENIGN" -gt 0 ]]; then
                log_warning "Found $NON_BENIGN non-benign error(s) in recent logs" "recent_errors" "$NON_BENIGN"
                if [[ "$VERBOSE" == "true" ]]; then
                    echo "$RECENT_LOGS" | grep '"level":"error"' | grep -v "is being terminated" | tail -5 | while read -r line; do
                        verbose "  $line"
                    done
                fi
            else
                verbose "Found $ERROR_LINES error(s) in recent logs, all benign (namespace terminating)"
            fi
        fi

        # Creation evidence for spot-checked clusters (from same log fetch)
        if [[ "$NEW_CD_COUNT" -gt 0 ]]; then
            CREATE_EVIDENCE=0
            for i in $(seq 0 $((SPOT_COUNT - 1))); do
                CD_NAME=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].metadata.name")
                CD_NS=$(echo "$SPOT_CHECK_CDS" | jq -r ".[$i].metadata.namespace")

                CLUSTER_LOGS=$(echo "$RECENT_LOGS" | grep "$CD_NS" || echo "")
                if [[ -n "$CLUSTER_LOGS" ]]; then
                    PD_SECRET_LOG=$(echo "$CLUSTER_LOGS" | grep "creating pd secret" | wc -l)
                    RECONCILE_LOG=$(echo "$CLUSTER_LOGS" | grep "Successfully reconciled" | wc -l)

                    if [[ "$PD_SECRET_LOG" -gt 0 || "$RECONCILE_LOG" -gt 0 ]]; then
                        verbose "Cluster $CD_NS/$CD_NAME: found creation log evidence (secret_create=$PD_SECRET_LOG, reconciled=$RECONCILE_LOG)"
                        CREATE_EVIDENCE=$((CREATE_EVIDENCE + 1))
                    else
                        verbose "Cluster $CD_NS/$CD_NAME: no creation evidence in recent logs (may have rotated)"
                    fi
                else
                    verbose "Cluster $CD_NS/$CD_NAME: no log entries in recent logs (may have rotated)"
                fi
            done

            if [[ "$CREATE_EVIDENCE" -gt 0 ]]; then
                log_success "Found creation evidence in logs for $CREATE_EVIDENCE of $SPOT_COUNT spot-checked cluster(s)" "creation_logs" "$CREATE_EVIDENCE"
            else
                verbose "No creation evidence in recent logs for spot-checked clusters (logs may have rotated)"
            fi
        fi
    fi

    # ------------------------------------------------------------------
    # Summary
    # ------------------------------------------------------------------
    log_info ""
    log_info "========================================="
    log_info "Functional Validation Summary"
    log_info "========================================="

    if [[ "$JSON_OUTPUT" == "true" ]]; then
        echo "{"
        echo "  \"namespace\": \"$NAMESPACE\","
        echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
        echo "  \"operator_start\": \"$POD_START\","
        echo "  \"operator_image\": \"$POD_IMAGE\","
        echo "  \"summary\": {"
        echo "    \"passed\": $CHECKS_PASSED,"
        echo "    \"failed\": $CHECKS_FAILED,"
        echo "    \"warnings\": $CHECKS_WARNING,"
        echo "    \"pdi_count\": $PDI_COUNT,"
        echo "    \"clusterdeployment_total\": $CD_TOTAL,"
        echo "    \"clusterdeployment_installed\": $CD_INSTALLED,"
        echo "    \"new_clusters_since_rollout\": $NEW_CD_COUNT,"
        echo "    \"pd_configmaps_total\": $PD_CM_TOTAL,"
        echo "    \"pd_secrets_total\": $PD_SECRET_TOTAL,"
        echo "    \"pd_syncsets_total\": $PD_SS_TOTAL"
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
        log_info "Operator: $POD_IMAGE (started $POD_START)"
        log_info ""
        log_info "Resources:"
        log_info "  PagerDutyIntegrations:        $PDI_COUNT"
        log_info "  ClusterDeployments:           $CD_TOTAL (installed: $CD_INSTALLED)"
        log_info "  New clusters since rollout:   $NEW_CD_COUNT"
        log_info "  Total PD ConfigMaps:          $PD_CM_TOTAL"
        log_info "  Total PD Secrets:             $PD_SECRET_TOTAL (existence only)"
        log_info "  Total PD SyncSets:            $PD_SS_TOTAL"
        log_info ""

        if [[ $CHECKS_FAILED -gt 0 ]]; then
            log_fail "Functional validation FAILED - $CHECKS_FAILED critical issues found" "overall" "N/A"
            exit 1
        elif [[ "$NEW_CD_COUNT" -eq 0 ]]; then
            log_warning "Functional validation INCOMPLETE - no new clusters to validate since rollout" "overall" "N/A"
            exit 0
        elif [[ $CHECKS_WARNING -gt 0 ]]; then
            log_warning "Functional validation PASSED with $CHECKS_WARNING warning(s)" "overall" "N/A"
            exit 0
        else
            log_success "Functional validation PASSED - All checks successful" "overall" "N/A"
            exit 0
        fi
    fi
}

main
