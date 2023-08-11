// This contains a minimal mock PagerDuty API for testing this package in
// ways that the GoMock version in mock_service.go.
// TODO: Build out this minimal mock so that the pagerdutyintegration package
// can utilize this instead of the GoMock variant as well.

package pagerduty

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	pd "github.com/PagerDuty/go-pagerduty"
	"github.com/openshift/pagerduty-operator/pkg/utils"
)

const (
	mockAlertId             string = "ALT1"
	mockAlertId2            string = "ALT2"
	mockEscalationPolicyId  string = "ESC1"
	mockEscalationPolicyId2 string = "ESC2"
	mockIncidentId          string = "INC1"
	mockIncidentId2         string = "INC2"
	mockIntegrationId       string = "INT1"
	mockIntegrationId2      string = "INT2"
	mockIntegrationId3      string = "INT3"
	mockIntegrationKey      string = "KEY1"
	mockIntegrationKey2     string = "KEY2"
	mockIntegrationKey3     string = "KEY3"
	mockServiceId           string = "SVC1"
	mockServiceId2          string = "SVC2"
)

type mockApi struct {
	Client *mockClient
	State  *mockState
	mux    *http.ServeMux
	server *httptest.Server
}

type mockState struct {
	Alerts             map[string][]*pd.IncidentAlert
	EscalationPolicies map[string]*pd.EscalationPolicy
	Incidents          []*pd.Incident
	Integrations       []*pd.Integration
	Services           map[string]*pd.Service
}

type mockClient struct {
	SvcClient
}

// defaultMockPagerdutyState returns a default mock state that's interesting for this operator
func defaultMockPagerdutyState() *mockState {
	return &mockState{
		Alerts: map[string][]*pd.IncidentAlert{
			mockIncidentId: {
				{
					APIObject:   pd.APIObject{ID: mockAlertId},
					Status:      "triggered",
					Service:     pd.APIObject{ID: mockServiceId},
					Incident:    pd.APIReference{ID: mockIncidentId},
					Integration: pd.APIObject{ID: mockIntegrationId},
				},
			},
			mockIncidentId2: {
				{
					APIObject:   pd.APIObject{ID: mockAlertId2},
					Status:      "resolved",
					Service:     pd.APIObject{ID: mockServiceId},
					Incident:    pd.APIReference{ID: mockIncidentId2},
					Integration: pd.APIObject{ID: mockIntegrationId2},
				},
			},
		},
		EscalationPolicies: map[string]*pd.EscalationPolicy{
			mockEscalationPolicyId: {
				APIObject: pd.APIObject{ID: mockEscalationPolicyId},
				Services: []pd.APIObject{
					{ID: mockServiceId},
				},
			},
			mockEscalationPolicyId2: {
				APIObject: pd.APIObject{ID: mockEscalationPolicyId2},
			},
		},
		Incidents: []*pd.Incident{
			{
				APIObject: pd.APIObject{ID: mockIncidentId},
				Service:   pd.APIObject{ID: mockServiceId},
				Status:    "triggered",
				AlertCounts: pd.AlertCounts{
					Triggered: 1,
					Resolved:  0,
					All:       1,
				},
			},
			{
				APIObject: pd.APIObject{ID: mockIncidentId2},
				Service:   pd.APIObject{ID: mockServiceId},
				Status:    "resolved",
				AlertCounts: pd.AlertCounts{
					Triggered: 0,
					Resolved:  1,
					All:       1,
				},
			},
		},
		Integrations: []*pd.Integration{
			{
				APIObject:      pd.APIObject{ID: mockIntegrationId},
				IntegrationKey: mockIntegrationKey,
				Service:        &pd.APIObject{ID: mockServiceId},
			},
			{
				APIObject:      pd.APIObject{ID: mockIntegrationId2},
				IntegrationKey: mockIntegrationKey2,
				Service:        &pd.APIObject{ID: mockServiceId},
			},
		},
		Services: map[string]*pd.Service{
			mockServiceId: {
				APIObject: pd.APIObject{ID: mockServiceId},
				Status:    "disabled",
				EscalationPolicy: pd.EscalationPolicy{
					APIObject: pd.APIObject{ID: mockEscalationPolicyId},
				},
				Integrations: []pd.Integration{
					{
						APIObject:      pd.APIObject{ID: mockIntegrationId},
						IntegrationKey: mockIntegrationKey,
						Service:        &pd.APIObject{ID: mockServiceId},
					},
					{
						APIObject:      pd.APIObject{ID: mockIntegrationId2},
						IntegrationKey: mockIntegrationKey2,
						Service:        &pd.APIObject{ID: mockServiceId},
					},
				},
			},
		},
	}
}

func defaultMockApi() *mockApi {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	mockApi := &mockApi{
		Client: newMockClient(server),
		State:  defaultMockPagerdutyState(),
		mux:    mux,
		server: server,
	}

	mockApi.setupCreateServiceHandler()
	mockApi.setupDefaultServiceHandlers()
	mockApi.setupDefaultGetIntegrationHandler()
	mockApi.setupDefaultGetEscalationPolicyHandler()
	mockApi.setupDefaultListIncidentsHandler()
	mockApi.setupDefaultListIncidentAlertsHandler()
	mockApi.setupV2EventsHandler()

	return mockApi
}

func (m *mockApi) cleanup() {
	m.server.Close()
}

// withTestHttpClient is meant to be used with httptest when testing to pass a mock http client
func withTestHttpClient(testClient *http.Client) pd.ClientOptions {
	return func(c *pd.Client) {
		c.HTTPClient = testClient
	}
}

func newMockClient(server *httptest.Server) *mockClient {
	return &mockClient{
		SvcClient{
			APIKey: "apiKey",
			PdClient: pd.NewClient(
				"apiKey",
				withTestHttpClient(server.Client()),
				pd.WithAPIEndpoint(server.URL),
				pd.WithV2EventsAPIEndpoint(server.URL),
			),
		},
	}
}

// setupDefaultServiceHandlers sets up handlers to get and update mock services
// as well as creating integrations for those services
func (m *mockApi) setupDefaultServiceHandlers() {
	for _, svc := range m.State.Services {
		m.setupCreateIntegrationHandler(svc.ID)

		m.mux.HandleFunc(fmt.Sprintf("/services/%s", svc.ID), func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				serviceData := map[string]pd.Service{
					"service": *svc,
				}
				resp, err := json.Marshal(serviceData)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, err = w.Write(resp)
				if err != nil {
					return
				}
			case http.MethodPut:
				// Update default mock service
				var serviceData map[string]pd.Service
				err := json.NewDecoder(r.Body).Decode(&serviceData)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				service, ok := serviceData["service"]
				if !ok {
					http.Error(w, "Could not find expected key: service", http.StatusBadRequest)
					return
				}
				service.ID = svc.ID
				m.State.Services[svc.ID] = &service
				processedService := map[string]pd.Service{
					"service": service,
				}

				resp, err := json.Marshal(processedService)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusCreated)
				_, err = w.Write(resp)
				if err != nil {
					return
				}
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		})
	}
}

// setupDefaultListIncidentsHandler sets up a handler to respond to listing incidents
func (m *mockApi) setupDefaultListIncidentsHandler() {
	var incidents []pd.Incident
	for _, inc := range m.State.Incidents {
		incidents = append(incidents, *inc)
	}

	incidentsData := map[string][]pd.Incident{
		"incidents": incidents,
	}

	m.mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		filteredIncidentsData := processListIncidentsQueryParams(r.URL.Query(), incidentsData)

		resp, err := json.Marshal(filteredIncidentsData)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(resp)
		if err != nil {
			return
		}
	})
}

// setupDefaultListIncidentAlertsHandler sets up a handler to respond to listing alerts
func (m *mockApi) setupDefaultListIncidentAlertsHandler() {
	for incidentId, alerts := range m.State.Alerts {
		// Convert []*pd.IncidentAlert to []pd.IncidentAlert
		var alertSlice []pd.IncidentAlert
		for _, alert := range alerts {
			alertSlice = append(alertSlice, *alert)
		}
		alertsData := map[string][]pd.IncidentAlert{
			"alerts": alertSlice,
		}

		m.mux.HandleFunc(fmt.Sprintf("/incidents/%s/alerts", incidentId), func(w http.ResponseWriter, r *http.Request) {
			filteredAlertsData := processListIncidentAlertsQueryParams(r.URL.Query(), alertsData)
			resp, err := json.Marshal(filteredAlertsData)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			_, err = w.Write(resp)
			if err != nil {
				return
			}
		})
	}
}

// setupCreateServiceHandler sets up a handler to respond to creating a service with ID mockServiceId2
func (m *mockApi) setupCreateServiceHandler() {
	m.mux.HandleFunc("/services", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var serviceData map[string]pd.Service

		err := json.NewDecoder(r.Body).Decode(&serviceData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		service, ok := serviceData["service"]
		if !ok {
			http.Error(w, "could not find expected key: service", http.StatusBadRequest)
			return
		}

		service.ID = mockServiceId2
		m.State.Services[service.ID] = &service
		m.setupCreateIntegrationHandler(service.ID)
		processedService := map[string]pd.Service{
			"service": service,
		}

		resp, err := json.Marshal(processedService)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write(resp)
		if err != nil {
			return
		}
	})
}

// setupDefaultGetEscalationPolicyHandler sets up a mock escalation policy to interact with since the PagerDuty operator
// expects an escalation policy to already exist.
func (m *mockApi) setupDefaultGetEscalationPolicyHandler() {
	for _, ep := range m.State.EscalationPolicies {
		escalationPolicyData := map[string]pd.EscalationPolicy{
			"escalation_policy": *ep,
		}

		m.mux.HandleFunc(fmt.Sprintf("/escalation_policies/%s", ep.ID), func(w http.ResponseWriter, r *http.Request) {
			resp, err := json.Marshal(escalationPolicyData)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, err = w.Write(resp)
			if err != nil {
				return
			}
		})
	}
}

// setupCreateIntegrationHandler sets up a handler to create new integrations for a provided service ID
func (m *mockApi) setupCreateIntegrationHandler(serviceId string) {
	m.mux.HandleFunc(fmt.Sprintf("/services/%s/integrations", serviceId), func(w http.ResponseWriter, r *http.Request) {
		var integrationData map[string]pd.Integration

		err := json.NewDecoder(r.Body).Decode(&integrationData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		integration, ok := integrationData["integration"]
		if !ok {
			http.Error(w, "Could not find expected key: integration", http.StatusBadRequest)
			return
		}

		integration.ID = mockIntegrationId3
		integration.IntegrationKey = mockIntegrationKey3
		m.State.Integrations = append(m.State.Integrations, &integration)
		processedIntegration := map[string]pd.Integration{
			"integration": integration,
		}

		resp, err := json.Marshal(processedIntegration)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write(resp)
		if err != nil {
			return
		}
	})
}

// setupDefaultGetIntegrationHandler sets up a handler for mocking get integration calls for a provided service
func (m *mockApi) setupDefaultGetIntegrationHandler() {
	for _, svc := range m.State.Services {
		for _, integration := range svc.Integrations {
			if integration.APIObject.ID == "" {
				panic("integration is missing required field: ID")
			}
			integrationData := map[string]pd.Integration{
				"integration": integration,
			}

			m.mux.HandleFunc(fmt.Sprintf("/services/%s/integrations/%s", svc.APIObject.ID, integration.APIObject.ID), func(w http.ResponseWriter, r *http.Request) {
				resp, err := json.Marshal(integrationData)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, err = w.Write(resp)
				if err != nil {
					return
				}
			})
		}
	}
}

func (m *mockApi) setupV2EventsHandler() {
	success := pd.V2EventResponse{
		Status:   "success",
		DedupKey: "",
		Message:  "Event processed",
		Errors:   nil,
	}

	failure := pd.V2EventResponse{
		Status:  "Unrecognized object",
		Message: "Event object format is unrecognized",
		Errors:  []string{"JSON parse error"},
	}

	m.mux.HandleFunc("/v2/enqueue", func(w http.ResponseWriter, r *http.Request) {
		var eventReq pd.V2Event
		err := json.NewDecoder(r.Body).Decode(&eventReq)
		if err != nil || eventReq.RoutingKey == "" || eventReq.Action == "" {
			w.WriteHeader(http.StatusInternalServerError)
			resp, err := json.Marshal(failure)
			if err != nil {
				return
			}
			if _, err = w.Write(resp); err != nil {
				return
			}
			return
		}

		// TODO: Actually resolve the alerts
		success.DedupKey = eventReq.DedupKey
		resp, err := json.Marshal(success)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err = w.Write(resp); err != nil {
			return
		}
	})
}

// processListIncidentAlertsQueryParams filters the list of alerts to return based on query params
// currently only "statuses" is supported
func processListIncidentAlertsQueryParams(queries map[string][]string, alerts map[string][]pd.IncidentAlert) map[string][]pd.IncidentAlert {
	statuses := queries["statuses[]"]

	if len(statuses) == 0 {
		return alerts
	}

	filteredAlerts := map[string][]pd.IncidentAlert{
		"alerts": {},
	}

	for _, alert := range alerts["alerts"] {
		if utils.Contains(statuses, alert.Status) {
			filteredAlerts["alerts"] = append(filteredAlerts["alerts"], alert)
		}
	}

	return filteredAlerts
}

// processListIncidentsQueryParams filters the list of incidents to return based on query params
// currently only "service_ids" and "statuses" are supported
func processListIncidentsQueryParams(queries map[string][]string, incidents map[string][]pd.Incident) map[string][]pd.Incident {
	serviceIds := queries["service_ids[]"]
	statuses := queries["statuses[]"]

	if len(serviceIds) == 0 && len(statuses) == 0 {
		return incidents
	}

	filteredIncidents := map[string][]pd.Incident{
		"incidents": {},
	}

	for _, inc := range incidents["incidents"] {
		if utils.Contains(serviceIds, inc.Service.ID) && utils.Contains(statuses, inc.Status) {
			filteredIncidents["incidents"] = append(filteredIncidents["incidents"], inc)
		}
	}

	return filteredIncidents
}
