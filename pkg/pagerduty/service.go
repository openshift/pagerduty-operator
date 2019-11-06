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

func getSecretKey(data map[string][]byte, key string) (string, error) {
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

//Client is a wrapper interface for the svcClient to allow for easier testing
type Client interface {
	GetService(data *Data) (*pdApi.Service, error)
	GetIntegrationKey(data *Data) (string, error)
	CreateService(data *Data) (string, error)
	DeleteService(data *Data) error
}

//svcClient wraps pdApi.Client
type svcClient struct {
	APIKey   string
	pdClient *pdApi.Client
}

//NewClient creates out client wrapper object for the actual pdApi.Client we use.
func NewClient(APIKey string) Client {
	return &svcClient{
		APIKey:   APIKey,
		pdClient: pdApi.NewClient(APIKey),
	}
}

// Data describes the data that is needed for PagerDuty api calls
type Data struct {
	escalationPolicyID string
	autoResolveTimeout uint
	acknowledgeTimeOut uint
	servicePrefix      string
	APIKey             string
	ClusterID          string
	BaseDomain         string

	ServiceID     string
	IntegrationID string
}

// ParsePDConfig parses the PD secret and stores it in the struct
func (data *Data) ParsePDConfig(osc client.Client) error {

	pdAPISecret := &corev1.Secret{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: config.OperatorNamespace, Name: config.PagerDutyAPISecretName}, pdAPISecret)
	if err != nil {
		return err
	}

	data.APIKey, err = getSecretKey(pdAPISecret.Data, config.PagerDutyAPISecretKey)
	if err != nil {
		return err
	}

	data.escalationPolicyID, err = getSecretKey(pdAPISecret.Data, "ESCALATION_POLICY")
	if err != nil {
		return err
	}

	autoResolveTimeoutStr, err := getSecretKey(pdAPISecret.Data, "RESOLVE_TIMEOUT")
	if err != nil {
		return err
	}
	data.autoResolveTimeout, err = convertStrToUint(autoResolveTimeoutStr)
	if err != nil {
		return err
	}

	acknowledgeTimeStr, err := getSecretKey(pdAPISecret.Data, "ACKNOWLEDGE_TIMEOUT")
	if err != nil {
		return err
	}
	data.acknowledgeTimeOut, err = convertStrToUint(acknowledgeTimeStr)
	if err != nil {
		return err
	}

	data.servicePrefix, err = getSecretKey(pdAPISecret.Data, "SERVICE_PREFIX")
	if err != nil {
		data.servicePrefix = "osd"
	}

	return nil
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
func (c *svcClient) GetService(data *Data) (*pdApi.Service, error) {
	service, err := c.pdClient.GetService(data.ServiceID, nil)
	if err != nil {
		return nil, err
	}

	return service, nil
}

// GetIntegrationKey searches the PD API for an already existing service and returns the first integration key
func (c *svcClient) GetIntegrationKey(data *Data) (string, error) {
	integration, err := c.pdClient.GetIntegration(data.ServiceID, data.IntegrationID, pdApi.GetIntegrationOptions{})
	if err != nil {
		return "", err
	}

	return integration.IntegrationKey, nil
}

// CreateService creates a service in pagerduty for the specified clusterid and returns the service key
func (c *svcClient) CreateService(data *Data) (string, error) {

	escalationPolicy, err := c.pdClient.GetEscalationPolicy(string(data.escalationPolicyID), nil)
	if err != nil {
		return "", errors.New("Escalation policy not found in PagerDuty")
	}

	clusterService := pdApi.Service{
		Name:                   data.servicePrefix + "-" + data.ClusterID + "." + data.BaseDomain + "-hive-cluster",
		Description:            data.ClusterID + " - A managed hive created cluster",
		EscalationPolicy:       *escalationPolicy,
		AutoResolveTimeout:     &data.autoResolveTimeout,
		AcknowledgementTimeout: &data.acknowledgeTimeOut,
		AlertCreation:          "create_alerts_and_incidents",
		IncidentUrgencyRule: &pdApi.IncidentUrgencyRule{
			Type:    "constant",
			Urgency: config.PagerDutyUrgencyRule,
		},
	}

	var newSvc *pdApi.Service
	newSvc, err = c.pdClient.CreateService(clusterService)
	if err != nil {
		if !strings.Contains(err.Error(), "Name has already been taken") {
			return "", err
		}
		lso := pdApi.ListServiceOptions{}
		lso.Query = clusterService.Name
		currentSvcs, newerr := c.pdClient.ListServices(lso)
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

	clusterIntegration := pdApi.Integration{
		Name: "V4 Alertmanager",
		Type: "events_api_v2_inbound_integration",
	}

	newInt, err := c.pdClient.CreateIntegration(newSvc.ID, clusterIntegration)
	if err != nil {
		return "", err
	}
	data.IntegrationID = newInt.ID

	return data.IntegrationID, nil
}

// DeleteService will get a service from the PD api and delete it
func (c *svcClient) DeleteService(data *Data) error {
	err := c.pdClient.DeleteService(data.ServiceID)
	return err
}
