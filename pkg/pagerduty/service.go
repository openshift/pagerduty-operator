// Copyright 2019 RedHat
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pagerduty

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/openshift/pagerduty-operator/config"
	"github.com/openshift/pagerduty-operator/pkg/localmetrics"

	"time"

	pdApi "github.com/PagerDuty/go-pagerduty"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getConfigMapKey(data map[string]string, key string) (string, error) {
	if _, ok := data[key]; !ok {
		errorStr := fmt.Sprintf("%v does not exist", key)
		return "", errors.New(errorStr)
	}
	retString := data[key]
	if len(retString) <= 0 {
		errorStr := fmt.Sprintf("%v is empty", key)
		return "", errors.New(errorStr)
	}
	return retString, nil
}

func GetSecretKey(data map[string][]byte, key string) (string, error) {
	if _, ok := data[key]; !ok {
		errorStr := fmt.Sprintf("%v does not exist", key)
		return "", errors.New(errorStr)
	}
	retString := string(data[key])
	if len(retString) <= 0 {
		errorStr := fmt.Sprintf("%v is empty", key)
		return "", errors.New(errorStr)
	}
	return retString, nil
}

//Client is a wrapper interface for the SvcClient to allow for easier testing
type Client interface {
	GetService(data *Data) (*pdApi.Service, error)
	GetIntegrationKey(data *Data) (string, error)
	CreateService(data *Data) (string, error)
	DeleteService(data *Data) error
	EnableService(data *Data) error
	DisableService(data *Data) error
	UpdateEscalationPolicy(data *Data) error
}

type PdClient interface {
	GetService(string, *pdApi.GetServiceOptions) (*pdApi.Service, error)
	GetEscalationPolicy(string, *pdApi.GetEscalationPolicyOptions) (*pdApi.EscalationPolicy, error)
	GetIntegration(string, string, pdApi.GetIntegrationOptions) (*pdApi.Integration, error)
	CreateService(service pdApi.Service) (*pdApi.Service, error)
	DeleteService(id string) error
	CreateIntegration(serviceID string, integration pdApi.Integration) (*pdApi.Integration, error)
	ListServices(pdApi.ListServiceOptions) (*pdApi.ListServiceResponse, error)
	ListIncidents(pdApi.ListIncidentsOptions) (*pdApi.ListIncidentsResponse, error)
	ListIncidentAlerts(incidentId string) (*pdApi.ListAlertsResponse, error)
	UpdateService(service pdApi.Service) (*pdApi.Service, error)
}

type ManageEventFunc func(pdApi.V2Event) (*pdApi.V2EventResponse, error)
type DelayFunc func(time.Duration)

//SvcClient wraps pdApi.Client
type SvcClient struct {
	APIKey      string
	PdClient    PdClient
	ManageEvent ManageEventFunc
	Delay       DelayFunc
}

type customHTTPClient struct {
	pdApi.HTTPClient
	controller string
}

// Do wrapping standard call to time it
func (c customHTTPClient) Do(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := c.HTTPClient.Do(req)

	if err == nil {
		localmetrics.AddAPICall(c.controller, req, resp, time.Since(start).Seconds())
	}

	return resp, err
}

// WithCustomHTTPClient allows to wrapper to monitor API response time
func WithCustomHTTPClient(controllerName string) pdApi.ClientOptions {
	return func(c *pdApi.Client) {
		c.HTTPClient = customHTTPClient{
			HTTPClient: c.HTTPClient,
			controller: controllerName,
		}
	}
}

//NewClient creates out client wrapper object for the actual pdApi.Client we use.
func NewClient(APIKey string, controllerName string) Client {
	return &SvcClient{
		APIKey:      APIKey,
		PdClient:    pdApi.NewClient(APIKey, WithCustomHTTPClient(controllerName)),
		ManageEvent: pdApi.ManageEvent,
		Delay:       time.Sleep,
	}
}

// Data describes the data that is needed for PagerDuty api calls
type Data struct {
	PDIEscalationPolicyID string
	AutoResolveTimeout    uint
	AcknowledgeTimeOut    uint
	ServicePrefix         string
	APIKey                string
	ClusterID             string
	BaseDomain            string

	ServiceID         string
	IntegrationID     string
	EsclationPolicyID string
	Hibernating       bool
	LimitedSupport    bool
}

// ParseClusterConfig parses the cluster specific config map and stores the IDs in the data struct
func (data *Data) ParseClusterConfig(osc client.Client, namespace string, cmName string) error {
	pdAPIConfigMap := &corev1.ConfigMap{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: cmName}, pdAPIConfigMap)
	if err != nil {
		return err
	}

	data.ServiceID, err = getConfigMapKey(pdAPIConfigMap.Data, "SERVICE_ID")
	if err != nil {
		return err
	}

	data.IntegrationID, err = getConfigMapKey(pdAPIConfigMap.Data, "INTEGRATION_ID")
	if err != nil {
		return err
	}

	data.EsclationPolicyID, err = getConfigMapKey(pdAPIConfigMap.Data, "ESCALATION_POLICY_ID")
	if err != nil {
		return err
	}

	val := pdAPIConfigMap.Data["HIBERNATING"]
	data.Hibernating = val == "true"

	isInLimitedSupport := pdAPIConfigMap.Data["LIMITED_SUPPORT"]
	data.LimitedSupport = isInLimitedSupport == "true"

	return nil
}

// ParseClusterConfig parses the cluster specific config map and stores the IDs in the data struct
func (data *Data) SetClusterConfig(osc client.Client, namespace string, cmName string) error {
	pdAPIConfigMap := &corev1.ConfigMap{}
	if err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: cmName}, pdAPIConfigMap); err != nil {
		return err
	}

	pdAPIConfigMap.Data["SERVICE_ID"] = data.ServiceID
	pdAPIConfigMap.Data["INTEGRATION_ID"] = data.IntegrationID
	pdAPIConfigMap.Data["ESCALATION_POLICY_ID"] = data.EsclationPolicyID
	pdAPIConfigMap.Data["HIBERNATING"] = strconv.FormatBool(data.Hibernating)
	pdAPIConfigMap.Data["LIMITED_SUPPORT"] = strconv.FormatBool(data.LimitedSupport)

	if err := osc.Update(context.TODO(), pdAPIConfigMap); err != nil {
		return err
	}

	return nil
}

// GetService searches the PD API for an already existing service
func (c *SvcClient) GetService(data *Data) (*pdApi.Service, error) {
	service, err := c.PdClient.GetService(data.ServiceID, nil)
	if err != nil {
		return nil, err
	}

	return service, nil
}

// GetIntegrationKey searches the PD API for an already existing service and returns the first integration key
func (c *SvcClient) GetIntegrationKey(data *Data) (string, error) {
	integration, err := c.PdClient.GetIntegration(data.ServiceID, data.IntegrationID, pdApi.GetIntegrationOptions{})
	if err != nil {
		return "", err
	}

	return integration.IntegrationKey, nil
}

// CreateService creates a service in pagerduty for the specified clusterid and returns the service key
func (c *SvcClient) CreateService(data *Data) (string, error) {
	escalationPolicy, err := c.PdClient.GetEscalationPolicy(string(data.PDIEscalationPolicyID), nil)
	if err != nil {
		return "", errors.New("Escalation policy not found in PagerDuty")
	}

	clusterService := pdApi.Service{
		Name:                   generatePDServiceName(data),
		Description:            generatePDServiceDescription(data),
		EscalationPolicy:       *escalationPolicy,
		AutoResolveTimeout:     &data.AutoResolveTimeout,
		AcknowledgementTimeout: &data.AcknowledgeTimeOut,
		AlertCreation:          "create_alerts_and_incidents",
		IncidentUrgencyRule: &pdApi.IncidentUrgencyRule{
			Type:    "constant",
			Urgency: config.PagerDutyUrgencyRule,
		},
	}

	var newSvc *pdApi.Service
	newSvc, err = c.PdClient.CreateService(clusterService)
	if err != nil {
		if !strings.Contains(err.Error(), "Name has already been taken") {
			return "", err
		}
		lso := pdApi.ListServiceOptions{}
		lso.Query = clusterService.Name
		currentSvcs, newerr := c.PdClient.ListServices(lso)
		if newerr != nil {
			return "", err
		}

		if len(currentSvcs.Services) > 0 {
			for _, svc := range currentSvcs.Services {
				if svc.Name == clusterService.Name {
					newSvc = &svc
					break
				}
			}
		}

		if newSvc == nil {
			return "", err
		}
	}
	data.ServiceID = newSvc.ID

	data.IntegrationID, err = c.createIntegration(newSvc.ID, "V4 Alertmanager", "events_api_v2_inbound_integration")
	if err != nil {
		return "", err
	}

	data.EsclationPolicyID = newSvc.EscalationPolicy.ID

	return data.IntegrationID, err
}
func (c *SvcClient) createIntegration(serviceId, name, integrationType string) (string, error) {
	newIntegration := pdApi.Integration{
		Name: name,
		Type: integrationType,
	}

	newInt, err := c.PdClient.CreateIntegration(serviceId, newIntegration)
	if err != nil {
		return "", err
	}
	return newInt.ID, nil
}

// DeleteService will get a service from the PD api and delete it
func (c *SvcClient) DeleteService(data *Data) error {
	err := c.resolvePendingIncidents(data)
	if err != nil {
		return err
	}

	err = c.waitForIncidentsToResolve(data, 10*time.Second)
	if err != nil {
		return err
	}

	return c.PdClient.DeleteService(data.ServiceID)
}

// EnableService will set the PD service active
func (c *SvcClient) EnableService(data *Data) error {
	service, err := c.PdClient.GetService(data.ServiceID, nil)
	if err != nil {
		return err
	}

	if service.Status != "active" {
		service.Status = "active"
		_, err = c.PdClient.UpdateService(*service)
		return err
	}

	return nil
}

// DisableService will set the PD service disabled
func (c *SvcClient) DisableService(data *Data) error {
	service, err := c.PdClient.GetService(data.ServiceID, nil)
	if err != nil {
		return err
	}

	if err := c.resolvePendingIncidents(data); err != nil {
		return err
	}

	if err = c.waitForIncidentsToResolve(data, 10*time.Second); err != nil {
		return err
	}

	if service.Status != "disabled" {
		service.Status = "disabled"
		if _, err = c.PdClient.UpdateService(*service); err != nil {
			return err
		}
	}

	return nil
}

// UpdateEscalationPolicy will update the PD service escalation policy
func (c *SvcClient) UpdateEscalationPolicy(data *Data) error {
	escalationPolicy, err := c.PdClient.GetEscalationPolicy(data.PDIEscalationPolicyID, &pdApi.GetEscalationPolicyOptions{})
	if err != nil {
		return err
	}

	service, err := c.PdClient.GetService(data.ServiceID, nil)
	if err != nil {
		return err
	}

	service.EscalationPolicy.ID = escalationPolicy.ID

	_, err = c.PdClient.UpdateService(*service)
	if err != nil {
		return err
	}

	return nil
}

func (c *SvcClient) resolvePendingIncidents(data *Data) error {

	incidents, err := c.getIncidents(data)
	if err != nil {
		return err
	}

	if len(incidents) > 0 {
		serviceKey, err := c.GetIntegrationKey(data)
		if err != nil {
			return err
		}

		for _, incident := range incidents {
			alerts, err := c.PdClient.ListIncidentAlerts(incident.Id)
			if err != nil {
				return err
			}
			for _, alert := range alerts.Alerts {
				err = c.resolveIncident(serviceKey, alert.AlertKey)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (c *SvcClient) getIncidents(data *Data) ([]pdApi.Incident, error) {
	listServiceIncidentOptions := pdApi.ListIncidentsOptions{}
	listServiceIncidentOptions.ServiceIDs = []string{data.ServiceID}

	incidentsRes, err := c.PdClient.ListIncidents(listServiceIncidentOptions)
	if err != nil {
		return []pdApi.Incident{}, err
	}
	return incidentsRes.Incidents, err
}

// waitForIncidentsToResolve checks if all incidents have been resolved every 2 seconds,
// waiting for a maximum of maxWait
func (c *SvcClient) waitForIncidentsToResolve(data *Data, maxWait time.Duration) error {
	waitStep := 2 * time.Second
	incidents, err := c.getIncidents(data)
	if err != nil {
		return err
	}

	totalIncidents := len(incidents)

	start := time.Now()
	for _, incident := range incidents {
		if time.Since(start) > maxWait {
			return fmt.Errorf("timed out waiting for %d incidents to resolve, %d left: %v",
				totalIncidents,
				len(incidents),
				parseIncidentNumbers(incidents),
			)
		}

		if incident.AlertCounts.Triggered > 0 {
			c.Delay(waitStep)
			incidents, err = c.getIncidents(data)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// parseIncidentNumbers returns a slice of PagerDuty incident numbers
func parseIncidentNumbers(incidents []pdApi.Incident) []uint {
	var incidentNumbers []uint
	for _, incident := range incidents {
		incidentNumbers = append(incidentNumbers, incident.IncidentNumber)
	}

	return incidentNumbers
}

// generateServiceName checks if FedRamp is enabled. If it is, it returns
// an anonymized PD service name.
func generatePDServiceName(data *Data) string {
	if config.IsFedramp() {
		return data.ServicePrefix + "-" + data.ClusterID
	} else {
		return data.ServicePrefix + "-" + data.ClusterID + "." + data.BaseDomain + "-hive-cluster"
	}
}

// generateServiceDescription checks if FedRamp is enabled. If it is, it returns
// an empty PD service description
func generatePDServiceDescription(data *Data) string {
	if config.IsFedramp() {
		return ""
	} else {
		return data.ClusterID + " - A managed hive created cluster"
	}
}

func (c *SvcClient) resolveIncident(serviceKey, incidentKey string) error {
	event := pdApi.V2Event{}
	event.Payload = &pdApi.V2Payload{}
	event.RoutingKey = serviceKey
	event.Action = "resolve"
	event.DedupKey = incidentKey
	event.Payload.Summary = "Cluster does not exist anymore"
	event.Payload.Source = "pagerduty-operator"
	event.Payload.Severity = "info"
	_, err := c.ManageEvent(event)
	return err
}
