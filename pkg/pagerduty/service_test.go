package pagerduty_test

import (
	"testing"
	"time"

	pdApi "github.com/PagerDuty/go-pagerduty"
	"github.com/golang/mock/gomock"
	s "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	mockpd "github.com/openshift/pagerduty-operator/pkg/pagerduty/mock"
	"github.com/stretchr/testify/mock"
	"gotest.tools/assert"
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

func NewTestClient(t *testing.T) (s.Client, *mockpd.MockPdClient, *functionMock) {
	mockClient := mockpd.NewMockPdClient(gomock.NewController(t))
	funcMock := new(functionMock)
	return &s.SvcClient{
			APIKey:      "test-key",
			PdClient:    mockClient,
			ManageEvent: func(ev pdApi.V2Event) (*pdApi.V2EventResponse, error) { return funcMock.manageEvents(ev) },
			Delay:       func(d time.Duration) { funcMock.delay(d) },
		},
		mockClient,
		funcMock
}

func NewPdData() *s.Data {
	return &s.Data{
		APIKey:        "test-api-key",
		ClusterID:     "test-cluster-id",
		BaseDomain:    "test.domain",
		ServiceID:     "test-service-id",
		IntegrationID: "test-integration-id",
	}
}

func TestDeleteServiceNoPendingIncidents(t *testing.T) {
	c, mockPdClient, _ := NewTestClient(t)
	mockPdClient.EXPECT().ListIncidents(gomock.Any()).Return(&pdApi.ListIncidentsResponse{}, nil).Times(2)
	mockPdClient.EXPECT().DeleteService(gomock.Any()).Return(nil).Times(1)
	err := c.DeleteService(NewPdData())
	assert.Assert(t, err, nil, "Unexpected error occured")
}

func setupMockWithIncidents(mockPdClient *mockpd.MockPdClient, funcMock *functionMock, eventDelay int) {
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

func TestDeleteServiceTwoPendingIncidents(t *testing.T) {
	c, mockPdClient, funcMock := NewTestClient(t)
	setupMockWithIncidents(mockPdClient, funcMock, 1)
	err := c.DeleteService(NewPdData())
	assert.Equal(t, err, nil, "Unexpected error occured")
	funcMock.AssertNumberOfCalls(t, "manageEvents", 2)
}

func TestDeleteServiceTwoPendingIncidentsResolveDelayed(t *testing.T) {
	c, mockPdClient, funcMock := NewTestClient(t)
	setupMockWithIncidents(mockPdClient, funcMock, 3)
	funcMock.On("delay").Times(2)
	err := c.DeleteService(NewPdData())
	assert.Equal(t, err, nil, "Unexpected error occured")
	funcMock.AssertNumberOfCalls(t, "manageEvents", 2)
	funcMock.AssertNumberOfCalls(t, "delay", 2)
}

func TestDeleteServiceTwoPendingIncidentsResolveTimeout(t *testing.T) {
	c, mockPdClient, funcMock := NewTestClient(t)
	setupMockWithIncidents(mockPdClient, funcMock, 10)
	funcMock.On("delay").Times(5)
	err := c.DeleteService(NewPdData())
	assert.Error(t, err, "Incidents still pending")
	funcMock.AssertNumberOfCalls(t, "manageEvents", 2)
	funcMock.AssertNumberOfCalls(t, "delay", 5)
}
