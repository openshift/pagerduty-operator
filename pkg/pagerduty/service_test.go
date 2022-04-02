package pagerduty

import (
	"testing"
	"time"

	pdApi "github.com/PagerDuty/go-pagerduty"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type functionMock struct {
	mock.Mock
}

func (m *functionMock) manageEvents(pdApi.V2Event) (*pdApi.V2EventResponse, error) {
	args := m.Called()
	return args.Get(0).(*pdApi.V2EventResponse), args.Error(1)
}

func (m *functionMock) delay(d time.Duration) {
	m.Called()
}

func NewTestClient(t *testing.T) (Client, *MockPdClient, *functionMock) {
	mockClient := NewMockPdClient(gomock.NewController(t))
	funcMock := new(functionMock)
	return &SvcClient{
			APIKey:      "test-key",
			PdClient:    mockClient,
			ManageEvent: func(ev pdApi.V2Event) (*pdApi.V2EventResponse, error) { return funcMock.manageEvents(ev) },
			Delay:       func(d time.Duration) { funcMock.delay(d) },
		},
		mockClient,
		funcMock
}

func NewPdData() *Data {
	return &Data{
		APIKey:        "test-api-key",
		ClusterID:     "test-cluster-id",
		BaseDomain:    "test.domain",
		ServiceID:     "test-service-id",
		IntegrationID: "test-integration-id",
	}
}

func setupMockWithIncidents(mockPdClient *MockPdClient, funcMock *functionMock, eventDelay int) {
	incidentsResponse := &pdApi.ListIncidentsResponse{
		Incidents: []pdApi.Incident{
			incident("test-incident-1", 1),
			incident("test-incident-2", 1),
		},
	}
	incidentsResponseResolved := &pdApi.ListIncidentsResponse{
		Incidents: []pdApi.Incident{
			incident("test-incident-1", 0),
			incident("test-incident-2", 0),
		},
	}
	integration := &pdApi.Integration{
		IntegrationKey: "test-integration-key",
	}
	alert1 := &pdApi.ListAlertsResponse{
		Alerts: []pdApi.IncidentAlert{{}},
	}
	alert2 := &pdApi.ListAlertsResponse{
		Alerts: []pdApi.IncidentAlert{{}},
	}
	mockPdClient.EXPECT().ListIncidents(gomock.Any()).Return(incidentsResponse, nil).Times(eventDelay)
	mockPdClient.EXPECT().GetIntegration("test-service-id", "test-integration-id", gomock.Any()).Return(integration, nil).Times(1)
	mockPdClient.EXPECT().ListIncidents(gomock.Any()).Return(incidentsResponseResolved, nil).Times(1)
	mockPdClient.EXPECT().ListIncidentAlerts("test-incident-1").Return(alert1, nil).Times(1)
	mockPdClient.EXPECT().ListIncidentAlerts("test-incident-2").Return(alert2, nil).Times(1)
	mockPdClient.EXPECT().DeleteService(gomock.Any()).Return(nil).Times(1)
	funcMock.On("manageEvents").Return(&pdApi.V2EventResponse{}, nil).Times(2)
}

func incident(name string, triggeredCount uint) pdApi.Incident {
	return pdApi.Incident{
		Id: name,
		AlertCounts: pdApi.AlertCounts{
			Triggered: triggeredCount,
		},
	}

}

func TestDeleteService(t *testing.T) {
	tests := []struct {
		name             string
		eventDelay       int
		initialDelay     int
		expectError      bool
		expectedNumCalls int
	}{
		{
			name:             "Two pending incidents",
			eventDelay:       1,
			initialDelay:     0,
			expectError:      false,
			expectedNumCalls: 2,
		},
	}

	for _, test := range tests {
		c, mockPdClient, funcMock := NewTestClient(t)
		setupMockWithIncidents(mockPdClient, funcMock, test.eventDelay)
		funcMock.On("delay").Times(test.initialDelay)
		err := c.DeleteService(NewPdData())
		if test.expectError {
			assert.NotNilf(t, err, "expected '%s' to return an error", test.name)
		} else {
			assert.Nilf(t, err, "expected '%s' to return nil", test.name)
		}

		funcMock.AssertNumberOfCalls(t, "manageEvents", test.expectedNumCalls)
		funcMock.AssertNumberOfCalls(t, "delay", test.initialDelay)
	}
}
