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
	"strconv"
	"strings"

	"github.com/openshift/pagerduty-operator/config"

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

func convertStrToUint(value string) (uint, error) {
	var retVal uint

	parsedU64, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return retVal, err
	}
	retVal = uint(parsedU64)

	return retVal, nil
}

//Client is a wrapper interface for the SvcClient to allow for easier testing
type Client interface {
	GetService(data *Data) (*pdApi.Service, error)
	GetIntegrationKey(data *Data) (string, error)
	CreateService(data *Data) (string, error)
	DeleteService(data *Data) error
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

//NewClient creates out client wrapper object for the actual pdApi.Client we use.
func NewClient(APIKey string) Client {
	return &SvcClient{
		APIKey:      APIKey,
		PdClient:    pdApi.NewClient(APIKey),
		ManageEvent: pdApi.ManageEvent,
		Delay:       time.Sleep,
	}
}

// Data describes the data that is needed for PagerDuty api calls
type Data struct {
	EscalationPolicyID string
	AutoResolveTimeout uint
	AcknowledgeTimeOut uint
	ServicePrefix      string
	APIKey             string
	ClusterID          string
	BaseDomain         string

	ServiceID     string
	IntegrationID string
}

// ParseClusterConfig parses the cluster specific config map and stores the IDs in the data struct
func (data *Data) ParseClusterConfig(osc client.Client, namespace string, name string) error {
	pdAPIConfigMap := &corev1.ConfigMap{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: name + config.ConfigMapPostfix}, pdAPIConfigMap)
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

	escalationPolicy, err := c.PdClient.GetEscalationPolicy(string(data.EscalationPolicyID), nil)
	if err != nil {
		return "", errors.New("Escalation policy not found in PagerDuty")
	}

	clusterService := pdApi.Service{
		Name:                   data.ServicePrefix + "-" + data.ClusterID + "." + data.BaseDomain + "-hive-cluster",
		Description:            data.ClusterID + " - A managed hive created cluster",
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

func (c *SvcClient) waitForIncidentsToResolve(data *Data, maxWait time.Duration) (err error) {
	waitStep := 2 * time.Second
	incidents, err := c.getIncidents(data)

OUTER:
	for i := 0; time.Duration(i)*waitStep < maxWait; i++ {
		for _, incident := range incidents {
			if incident.AlertCounts.Triggered > 0 {
				c.Delay(waitStep)
				incidents, err = c.getIncidents(data)
				continue OUTER
			}
		}
		break
	}
	return
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
