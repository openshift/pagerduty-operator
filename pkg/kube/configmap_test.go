package kube

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateConfigMap(t *testing.T) {
	tests := []struct {
		name                        string
		namespace                   string
		cmName                      string
		pdServiceID                 string
		pdIntegrationID             string
		pdEscalationPolicyID        string
		limitedSupport              bool
		serviceOrchestrationEnabled bool
		serviceOrchestrationRule    string
		alertGroupingType           string
		alertGroupingTimeout        uint
	}{
		{
			name:                        "Standard values",
			namespace:                   "test-ns",
			cmName:                      "test-pd-config",
			pdServiceID:                 "SVC123",
			pdIntegrationID:             "INT456",
			pdEscalationPolicyID:        "ESC789",
			limitedSupport:              false,
			serviceOrchestrationEnabled: true,
			serviceOrchestrationRule:    "applied",
			alertGroupingType:           "time",
			alertGroupingTimeout:        300,
		},
		{
			name:                        "Limited support enabled",
			namespace:                   "other-ns",
			cmName:                      "other-pd-config",
			pdServiceID:                 "SVC000",
			pdIntegrationID:             "INT000",
			pdEscalationPolicyID:        "ESC000",
			limitedSupport:              true,
			serviceOrchestrationEnabled: false,
			serviceOrchestrationRule:    "",
			alertGroupingType:           "",
			alertGroupingTimeout:        0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cm := GenerateConfigMap(
				test.namespace,
				test.cmName,
				test.pdServiceID,
				test.pdIntegrationID,
				test.pdEscalationPolicyID,
				test.limitedSupport,
				test.serviceOrchestrationEnabled,
				test.serviceOrchestrationRule,
				test.alertGroupingType,
				test.alertGroupingTimeout,
			)

			assert.Equal(t, test.cmName, cm.Name)
			assert.Equal(t, test.namespace, cm.Namespace)
			assert.Equal(t, test.pdServiceID, cm.Data["SERVICE_ID"])
			assert.Equal(t, test.pdIntegrationID, cm.Data["INTEGRATION_ID"])
			assert.Equal(t, test.pdEscalationPolicyID, cm.Data["ESCALATION_POLICY_ID"])
			assert.Equal(t, strconv.FormatBool(test.limitedSupport), cm.Data["LIMITED_SUPPORT"])
			assert.Equal(t, strconv.FormatBool(test.serviceOrchestrationEnabled), cm.Data["SERVICE_ORCHESTRATION_ENABLED"])
			assert.Equal(t, test.serviceOrchestrationRule, cm.Data["SERVICE_ORCHESTRATION_RULE_APPLIED"])
			assert.Equal(t, test.alertGroupingType, cm.Data["ALERT_GROUPING_TYPE"])
			assert.Equal(t, fmt.Sprintf("%d", test.alertGroupingTimeout), cm.Data["ALERT_GROUPING_TIMEOUT"])
		})
	}
}
