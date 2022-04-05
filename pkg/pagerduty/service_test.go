package pagerduty

import (
	"testing"

	pd "github.com/PagerDuty/go-pagerduty"
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

func TestGetSecretKey(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string][]byte
		key         string
		expected    string
		expectError bool
	}{
		{
			name: "Normal",
			data: map[string][]byte{
				"key": []byte("value"),
			},
			key:         "key",
			expected:    "value",
			expectError: false,
		},
		{
			name: "Empty",
			data: map[string][]byte{
				"key": []byte(""),
			},
			key:         "key",
			expectError: true,
		},
		{
			name:        "Does not exist",
			data:        map[string][]byte{},
			key:         "key",
			expectError: true,
		},
	}

	for _, test := range tests {
		actual, err := GetSecretKey(test.data, test.key)
		if test.expectError {
			assert.NotNil(t, err)
		} else {
			assert.Equal(t, test.expected, actual)
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

		var testData Data
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
	mockServices := []*pd.Service{
		{
			APIObject: pd.APIObject{
				ID: "1",
			},
			Name: "one",
		},
	}

	tests := []struct {
		serviceId string
		expectErr bool
	}{
		{
			serviceId: "1",
			expectErr: false,
		},
		{
			serviceId: "notfound",
			expectErr: true,
		},
	}

	mock, server, _ := setupMockWithServices(mockServices)

	for _, test := range tests {
		_, err := mock.GetService(&Data{ServiceID: test.serviceId})
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}

	server.Close()
}

func TestSvcClient_GetIntegrationKey(t *testing.T) {
	mockServices := []*pd.Service{
		{
			APIObject: pd.APIObject{
				ID: "1",
			},
			Name: "one",
			Integrations: []pd.Integration{
				{
					APIObject: pd.APIObject{
						ID: "1",
					},
					Name:           "oneIntegration",
					IntegrationKey: "oneIntegrationKey",
				},
			},
		},
	}

	tests := []struct {
		serviceId     string
		integrationId string
		expected      string
		expectErr     bool
	}{
		{
			serviceId:     "1",
			integrationId: "1",
			expected:      "oneIntegrationKey",
			expectErr:     false,
		},
		{
			serviceId:     "1",
			integrationId: "notfound",
			expectErr:     true,
		},
	}

	mock, server, _ := setupMockWithServices(mockServices)

	for _, test := range tests {
		actual, err := mock.GetIntegrationKey(&Data{ServiceID: test.serviceId, IntegrationID: test.integrationId})
		if test.expectErr {
			assert.NotNil(t, err)
			assert.Equal(t, test.expected, actual)
		} else {
			assert.Nil(t, err)
		}
	}

	server.Close()
}

func TestSvcClient_CreateIntegration(t *testing.T) {
	mockServices := []*pd.Service{
		{
			APIObject: pd.APIObject{
				ID: "1",
			},
			Name: "one",
		},
	}

	tests := []struct {
		serviceId       string
		integrationName string
		integrationType string
		expectErr       bool
	}{
		{
			serviceId:       "1",
			integrationName: "integrationName",
			integrationType: "events_api_v2_inbound_integration",
			expectErr:       false,
		},
		{
			serviceId: "2",
			expectErr: true,
		},
	}

	mock, server, _ := setupMockWithServices(mockServices)

	for _, test := range tests {
		actual, err := mock.createIntegration(test.serviceId, test.integrationName, test.integrationType)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, mockIntegrationId, actual)
		}
	}

	server.Close()
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
				PDIEscalationPolicyID: mockEscalationPolicyId,
				AutoResolveTimeout:    30,
				AcknowledgeTimeOut:    30,
				ServicePrefix:         "servicePrefix",
				ClusterID:             "clusterID",
				BaseDomain:            "baseDomain",
			},
			expectErr: false,
		},
		{
			name: "Can't find escalation policy",
			data: &Data{
				PDIEscalationPolicyID: "notfound",
			},
			expectErr: true,
		},
	}

	mock, server, _ := setupMock()

	for _, test := range tests {
		_, err := mock.CreateService(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}

	server.Close()
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

	mock, server, _ := setupMock()

	for _, test := range tests {
		err := mock.EnableService(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}

	server.Close()
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
				PDIEscalationPolicyID: mockEscalationPolicyId,
				ServiceID:             mockServiceId,
			},
			expectErr: false,
		},
		{
			name: "missing escalation policy",
			data: &Data{
				PDIEscalationPolicyID: "notfound",
			},
			expectErr: true,
		},
		{
			name: "missing service",
			data: &Data{
				PDIEscalationPolicyID: mockEscalationPolicyId,
				ServiceID:             "notfound",
			},
			expectErr: true,
		},
	}

	mock, server, _ := setupMock()
	for _, test := range tests {
		err := mock.UpdateEscalationPolicy(test.data)
		if test.expectErr {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}
	}

	server.Close()
}
