package pagerduty

import (
	"testing"

	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetConfigMapKey(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]string
		key         string
		expected    string
		expectError bool
	}{
		{
			name: "Normal",
			data: map[string]string{
				"key": "value",
			},
			key:         "key",
			expected:    "value",
			expectError: false,
		},
		{
			name: "Empty",
			data: map[string]string{
				"key": "",
			},
			key:         "key",
			expectError: true,
		},
		{
			name:        "Does not exist",
			data:        map[string]string{},
			key:         "key",
			expectError: true,
		},
	}

	for _, test := range tests {
		actual, err := getConfigMapKey(test.data, test.key)
		if test.expectError {
			assert.NotNil(t, err)
		} else {
			assert.Equal(t, test.expected, actual)
			assert.Nil(t, err)
		}
	}
}

func TestNewData(t *testing.T) {
	tests := []struct {
		name      string
		pdi       *pagerdutyv1alpha1.PagerDutyIntegration
		expectErr bool
	}{
		{
			name: "escalation policy defined",
			pdi: &pagerdutyv1alpha1.PagerDutyIntegration{
				Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{
					EscalationPolicy: mockEscalationPolicyId,
				},
			},
			expectErr: false,
		},
		{
			name: "escalation policy undefined",
			pdi: &pagerdutyv1alpha1.PagerDutyIntegration{
				Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{},
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		_, err := NewData(test.pdi, "clusterId", "baseDomain")
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestParseSetClusterConfig(t *testing.T) {
	tests := []struct {
		name                   string
		cmName                 string
		namespace              string
		data                   map[string]string
		expectedHibernating    bool
		expectedLimitedSupport bool
		expectErr              bool
	}{
		{
			name:      "working",
			cmName:    "cluster-pd-config",
			namespace: "namespace",
			data: map[string]string{
				"SERVICE_ID":           "abcd",
				"INTEGRATION_ID":       "abcd",
				"ESCALATION_POLICY_ID": "abcd",
			},
			expectedHibernating:    false,
			expectedLimitedSupport: false,
			expectErr:              false,
		},
		{
			name:      "missing escalation policy id",
			cmName:    "cluster-pd-config",
			namespace: "namespace",
			data: map[string]string{
				"SERVICE_ID":     "abcd",
				"INTEGRATION_ID": "abcd",
			},
			expectedHibernating:    false,
			expectedLimitedSupport: false,
			expectErr:              false,
		},
		{
			name:      "hibernating",
			cmName:    "cluster-pd-config",
			namespace: "namespace",
			data: map[string]string{
				"SERVICE_ID":           "abcd",
				"INTEGRATION_ID":       "abcd",
				"ESCALATION_POLICY_ID": "abcd",
				"HIBERNATING":          "true",
			},
			expectedHibernating:    true,
			expectedLimitedSupport: false,
			expectErr:              false,
		},
		{
			name:      "missing values",
			cmName:    "cluster-pd-config",
			namespace: "namespace",
			data: map[string]string{
				"SERVICE_ID": "abcd",
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      test.cmName,
				Namespace: test.namespace,
			},
			Data: test.data,
		}

		s := runtime.NewScheme()
		s.AddKnownTypes(v1.SchemeGroupVersion, &v1.ConfigMap{})
		client := fake.NewFakeClientWithScheme(s, cm)

		testData := Data{
			EscalationPolicyID: mockEscalationPolicyId,
		}
		parseErr := testData.ParseClusterConfig(client, test.namespace, test.cmName)

		if test.expectErr {
			assert.NotNil(t, parseErr)
		} else {
			assert.Nil(t, parseErr)
			assert.Equal(t, test.expectedHibernating, testData.Hibernating)
			assert.Equal(t, test.expectedLimitedSupport, testData.LimitedSupport)
		}

		setErr := testData.SetClusterConfig(client, test.namespace, test.cmName)
		assert.Nil(t, setErr)
	}
}

func TestSvcClient_GetService(t *testing.T) {
	tests := []struct {
		serviceId string
		expectErr bool
	}{
		{
			serviceId: mockServiceId,
			expectErr: false,
		},
		{
			serviceId: "notfound",
			expectErr: true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		_, err := mock.Client.GetService(&Data{ServiceID: test.serviceId})
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSvcClient_GetIntegrationKey(t *testing.T) {
	tests := []struct {
		serviceId     string
		integrationId string
		expected      string
		expectErr     bool
	}{
		{
			serviceId:     mockServiceId,
			integrationId: mockIntegrationId,
			expected:      mockIntegrationKey,
			expectErr:     false,
		},
		{
			serviceId:     mockServiceId,
			integrationId: "notfound",
			expectErr:     true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		actual, err := mock.Client.GetIntegrationKey(&Data{ServiceID: test.serviceId, IntegrationID: test.integrationId})
		if test.expectErr {
			assert.NotNil(t, err)
			assert.Equal(t, test.expected, actual)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSvcClient_CreateIntegration(t *testing.T) {
	tests := []struct {
		serviceId       string
		integrationName string
		integrationType string
		expectErr       bool
	}{
		{
			serviceId:       mockServiceId,
			integrationName: "integrationName",
			integrationType: "events_api_v2_inbound_integration",
			expectErr:       false,
		},
		{
			serviceId: "notfound",
			expectErr: true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		actual, err := mock.Client.createIntegration(test.serviceId, test.integrationName, test.integrationType)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			// mock always creates an integration with ID mockIntegrationId3 when successful
			assert.Equal(t, mockIntegrationId3, actual)
		}
	}
}

func TestSvcClient_CreateService(t *testing.T) {
	tests := []struct {
		name      string
		data      *Data
		expectErr bool
	}{
		{
			name: "Works",
			data: &Data{
				EscalationPolicyID: mockEscalationPolicyId,
				ResolveTimeout:     30,
				AcknowledgeTimeOut: 30,
				ServicePrefix:      "servicePrefix",
				ClusterID:          "clusterID",
				BaseDomain:         "baseDomain",
			},
			expectErr: false,
		},
		{
			name: "Can't find escalation policy",
			data: &Data{
				EscalationPolicyID: "notfound",
			},
			expectErr: true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		_, err := mock.Client.CreateService(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSvcClient_EnableService(t *testing.T) {
	tests := []struct {
		name      string
		data      *Data
		expectErr bool
	}{
		{
			name: "Works",
			data: &Data{
				ServiceID: mockServiceId,
			},
			expectErr: false,
		},
		{
			name: "Can't find service",
			data: &Data{
				ServiceID: "notfound",
			},
			expectErr: true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		err := mock.Client.EnableService(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSvcClient_UpdateEscalationPolicy(t *testing.T) {
	tests := []struct {
		name      string
		data      *Data
		expectErr bool
	}{
		{
			name: "normal",
			data: &Data{
				EscalationPolicyID: mockEscalationPolicyId2,
				ServiceID:          mockServiceId,
			},
			expectErr: false,
		},
		{
			name: "missing escalation policy",
			data: &Data{
				EscalationPolicyID: "notfound",
			},
			expectErr: true,
		},
		{
			name: "missing service",
			data: &Data{
				EscalationPolicyID: mockEscalationPolicyId,
				ServiceID:          "notfound",
			},
			expectErr: true,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		err := mock.Client.UpdateEscalationPolicy(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func TestSvcClient_GetUnresolvedIncidents(t *testing.T) {
	tests := []struct {
		name              string
		data              *Data
		expectedIncidents int
		expectErr         bool
	}{
		{
			name: "mockServiceId has two incidents, only one unresolved",
			data: &Data{
				ServiceID: mockServiceId,
			},
			expectedIncidents: 1,
			expectErr:         false,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		actual, err := mock.Client.getUnresolvedIncidents(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, test.expectedIncidents, len(actual))
		}

	}
}

func TestSvcClient_GetUnresolvedAlerts(t *testing.T) {
	tests := []struct {
		name           string
		incidentId     string
		expectedAlerts int
		expectErr      bool
	}{
		{
			name:           "mockIncidentId has one unresolved alert",
			incidentId:     mockIncidentId,
			expectedAlerts: 1,
			expectErr:      false,
		},
		{
			name:           "mockIncidentId2 has one resolved alert",
			incidentId:     mockIncidentId2,
			expectedAlerts: 0,
			expectErr:      false,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		actual, err := mock.Client.getUnresolvedAlerts(test.incidentId)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, test.expectedAlerts, len(actual))
		}

	}
}

func TestSvcClient_ResolvePendingIncidents(t *testing.T) {
	tests := []struct {
		name      string
		data      *Data
		expectErr bool
	}{
		{
			name: "Resolve mockServiceId incidents",
			data: &Data{
				ServiceID: mockServiceId,
			},
			expectErr: false,
		},
	}

	mock := defaultMockApi()
	defer mock.cleanup()

	for _, test := range tests {
		err := mock.Client.resolvePendingIncidents(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}
}
