// This contains a minimal mock PagerDuty API for testing this package in
// ways that the GoMock version in mock_service.go.
// TODO: Build out this minimal mock so that the pagerdutyintegration package
// can utilize this instead of the GoMock variant as well.

package pagerduty

import (
	"encoding/json"
	"fmt"
	pd "github.com/PagerDuty/go-pagerduty"
	"net/http"
	"net/http/httptest"
)

const (
	mockEscalationPolicyId string = "ESC1"
	mockIncidentId         string = "INC1"
	mockIntegrationId      string = "INT1"
	mockServiceId          string = "SVC1"
)

type mockSvcClient struct {
	SvcClient
}

// withTestHttpClient is meant to be used with httptest when testing to pass a mock http client
func withTestHttpClient(testClient *http.Client) pd.ClientOptions {
	return func(c *pd.Client) {
		c.HTTPClient = testClient
	}
}

func newMockClient(server *httptest.Server) *mockSvcClient {
	return &mockSvcClient{
		SvcClient{
			APIKey: "apiKey",
			PdClient: pd.NewClient(
				"apiKey",
				withTestHttpClient(server.Client()),
				pd.WithAPIEndpoint(server.URL),
			),
		},
	}
}

// setupMock creates a mock with behavior to respond to largely get/list requests on mock objects
// whose names are constants in this package, e.g. mockServiceId, mockIncidentId
func setupMock() (*mockSvcClient, *httptest.Server, *http.ServeMux) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	setupCreateServiceHandler(mux)
	setupDefaultServiceHandlers(mux)
	setupDefaultListIncidentsHandler(mux)
	setupDefaultGetEscalationPolicyHandler(mux)

	return newMockClient(server), server, mux
}

// setupMockWithServices extends the default mock with a slice of provided services
func setupMockWithServices(services []*pd.Service) (*mockSvcClient, *httptest.Server, *http.ServeMux) {
	mock, server, mux := setupMock()
	for _, service := range services {
		setupServiceHandlers(mux, *service)
	}

	return mock, server, mux
}

// setupDefaultServiceHandlers sets up handlers to get and update a mock service with ID mockServiceId
// as well as creating integrations for that service
func setupDefaultServiceHandlers(mux *http.ServeMux) {
	serviceData := map[string]pd.Service{
		"service": {
			APIObject: pd.APIObject{
				ID: mockServiceId,
			},
			Status: "disabled",
		},
	}
	mux.HandleFunc(fmt.Sprintf("/services/%s", mockServiceId), func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Get default mock service
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
			service.ID = mockServiceId
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

	setupCreateIntegrationHandler(mux, mockServiceId)
}

// setupDefaultListIncidentsHandler sets up a handler to respond to listing incidents for mockServiceId
func setupDefaultListIncidentsHandler(mux *http.ServeMux) {
	incidentsData := map[string][]pd.Incident{
		"incidents": {
			{
				Id:     mockIncidentId,
				Status: "triggered",
				Service: pd.APIObject{
					ID: mockServiceId,
				},
				AlertCounts: pd.AlertCounts{
					All:       2,
					Triggered: 1,
					Resolved:  1,
				},
				EscalationPolicy: pd.APIObject{
					ID: mockEscalationPolicyId,
				},
			},
		},
	}

	mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		resp, err := json.Marshal(incidentsData)
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

// setupCreateServiceHandler sets up a handler to respond to creating a service
func setupCreateServiceHandler(mux *http.ServeMux) {
	mux.HandleFunc("/services", func(w http.ResponseWriter, r *http.Request) {
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
		service.ID = mockServiceId
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
func setupDefaultGetEscalationPolicyHandler(mux *http.ServeMux) {
	escalationPolicyData := map[string]pd.EscalationPolicy{
		"escalation_policy": {
			APIObject: pd.APIObject{
				ID: mockIntegrationId,
			},
			Services: []pd.APIObject{
				{
					ID: mockServiceId,
				},
			},
		},
	}
	mux.HandleFunc(fmt.Sprintf("/escalation_policies/%s", mockEscalationPolicyId), func(w http.ResponseWriter, r *http.Request) {
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

// setupCreateIntegrationHandler sets up a handler to create new integrations for a provided service ID
func setupCreateIntegrationHandler(mux *http.ServeMux, serviceId string) {
	mux.HandleFunc(fmt.Sprintf("/services/%s/integrations", serviceId), func(w http.ResponseWriter, r *http.Request) {
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
		integration.ID = mockIntegrationId
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

// setupServiceHandlers sets up a handler to get a provided service as well as any integrations of the service
func setupServiceHandlers(mux *http.ServeMux, service pd.Service) {
	if service.APIObject.ID == "" {
		panic("service is missing required field: ID")
	}
	svcData := map[string]pd.Service{
		"service": service,
	}
	mux.HandleFunc(fmt.Sprintf("/services/%s", service.APIObject.ID), func(w http.ResponseWriter, r *http.Request) {
		resp, err := json.Marshal(svcData)
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

	setupIntegrationsHandlers(mux, service)
}

// setupIntegrationsHandlers sets up a handler for mocking get integration calls for a provided service
func setupIntegrationsHandlers(mux *http.ServeMux, service pd.Service) {
	setupCreateIntegrationHandler(mux, service.APIObject.ID)

	for _, integration := range service.Integrations {
		if integration.APIObject.ID == "" {
			panic("integration is missing required field: ID")
		}
		integrationData := map[string]pd.Integration{
			"integration": integration,
		}
		mux.HandleFunc(fmt.Sprintf("/services/%s/integrations/%s", service.APIObject.ID, integration.APIObject.ID), func(w http.ResponseWriter, r *http.Request) {
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
