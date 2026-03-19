SCRIPTS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
REASON ?=
NAMESPACE ?= pagerduty-operator
BASELINE_DIR ?= /tmp/baselines
VERBOSE ?= -v

# Hive cluster info from ocm backplane status (lazy-evaluated)
HIVE_NAME = $(shell ocm backplane status 2>/dev/null | awk '/Cluster Name:/ {print $$NF}')
HIVE_ID = $(shell ocm backplane status 2>/dev/null | awk '/Cluster ID:/ {print $$NF}')

default: validate

.PHONY: check-reason check-cluster help
.PHONY: baseline compare validate validate-deployment validate-functional validate-metrics

help: ## Show this help
	@echo "PagerDuty Operator Validation"
	@echo ""
	@echo "Prerequisites:"
	@echo "  - Must be logged into a hive cluster via ocm backplane"
	@echo "  - REASON must be set to a Jira ticket URL for elevation"
	@echo ""
	@echo "Usage:"
	@echo "  make -f scripts/validate.mk <target> REASON=https://redhat.atlassian.net/browse/SREP-XXXX"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-25s %s\n", $$1, $$2}'

check-reason:
ifndef REASON
	$(error REASON is required. Set REASON to a Jira ticket URL, e.g. REASON=https://redhat.atlassian.net/browse/SREP-XXXX)
endif

check-cluster:
	@oc whoami --show-server >/dev/null 2>&1 || (echo "ERROR: Not logged into a cluster. Run 'ocm backplane login <hive>' first." && exit 1)
	@echo "Logged into: $(HIVE_ID) - $(HIVE_NAME)"

# --- Baseline targets ---

baseline: check-reason check-cluster ## Capture metrics baseline for current hive (saves to BASELINE_DIR)
	@mkdir -p $(BASELINE_DIR)
	@echo "Capturing baseline for $(HIVE_NAME)..."
	@bash $(SCRIPTS_DIR)/compare-metrics.sh --baseline -t "$(REASON)" $(VERBOSE) > $(BASELINE_DIR)/baseline-$(HIVE_NAME)-$(shell date +%Y%m%d).json
	@echo "Baseline saved to: $(BASELINE_DIR)/baseline-$(HIVE_NAME)-$(shell date +%Y%m%d).json"

# --- Compare target ---

compare: check-reason check-cluster ## Compare current metrics against baseline (set BASELINE=<file>)
ifndef BASELINE
	$(error BASELINE is required. Set BASELINE to a baseline JSON file, e.g. BASELINE=/tmp/baselines/baseline-hive-stage-01-20260318.json)
endif
	bash $(SCRIPTS_DIR)/compare-metrics.sh --compare "$(BASELINE)" -t "$(REASON)" $(VERBOSE)

# --- Validation targets ---

validate-deployment: check-reason check-cluster ## Validate operator deployment health
	bash $(SCRIPTS_DIR)/validate-deployment.sh -t "$(REASON)" $(VERBOSE)

validate-functional: check-reason check-cluster ## Validate operator functional behavior (PD resources for new clusters)
	bash $(SCRIPTS_DIR)/validate-functional.sh -t "$(REASON)" $(VERBOSE)

validate-metrics: check-reason check-cluster ## Validate operator Prometheus metrics
	bash $(SCRIPTS_DIR)/check-metrics.sh -t "$(REASON)" $(VERBOSE)

validate: check-reason check-cluster ## Run all three validations (deployment, functional, metrics)
	@echo "========================================="
	@echo "PagerDuty Operator Validation Suite"
	@echo "Cluster: $(HIVE_ID) - $(HIVE_NAME)"
	@echo "Reason:  $(REASON)"
	@echo "========================================="
	@echo ""
	@echo "--- Deployment Validation ---"
	@bash $(SCRIPTS_DIR)/validate-deployment.sh -t "$(REASON)" $(VERBOSE) || true
	@echo ""
	@echo "--- Functional Validation ---"
	@bash $(SCRIPTS_DIR)/validate-functional.sh -t "$(REASON)" $(VERBOSE) || true
	@echo ""
	@echo "--- Metrics Validation ---"
	@bash $(SCRIPTS_DIR)/check-metrics.sh -t "$(REASON)" $(VERBOSE) || true
	@echo ""
	@echo "========================================="
	@echo "Validation complete."
	@echo "========================================="
