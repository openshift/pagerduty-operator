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

	pdApi "github.com/PagerDuty/go-pagerduty"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// Data describes the data that is needed for PagerDuty api calls
type Data struct {
	escalationPolicyID string
	autoResolveTimeout uint
	acknowledgeTimeOut uint
	servicePrefix      string
	APIKey             string
	ClusterID          string
	BaseDomain         string
}

// ParsePDConfig parses the PD Config map and stores it in the struct
func (data *Data) ParsePDConfig(osc client.Client) error {

	pdAPISecret := &corev1.Secret{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: "pagerduty-operator", Name: "pagerduty-api-key"}, pdAPISecret)
	if err != nil {
		return err
	}

	data.APIKey, err = getSecretKey(pdAPISecret.Data, "PAGERDUTY_API_KEY")
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

// GetService searches the PD API for an already existing service
func (data *Data) GetService() (string, error) {
	client := pdApi.NewClient(data.APIKey)
	var opts pdApi.ListServiceOptions
	opts.Query = data.servicePrefix + "-" + data.ClusterID + "-" + data.BaseDomain + "-hive-cluster"
	services, err := client.ListServices(opts)
	if err != nil {
		return "", err
	}

	if len(services.Services) <= 0 {
		return "", errors.New("No services returned from PagerDuty")
	} else if len(services.Services) > 1 {
		return "", errors.New("Multiple services returned from PagerDuty")
	}

	return services.Services[0].ID, nil
}

// CreateService creates a service in pagerduty for the specified clusterid and returns the service key
func (data *Data) CreateService() (string, error) {
	client := pdApi.NewClient(data.APIKey)

	escalationPolicy, err := client.GetEscalationPolicy(string(data.escalationPolicyID), nil)
	if err != nil {
		return "", errors.New("Escalation policy not found in PagerDuty")
	}

	clusterService := pdApi.Service{
		Name:                   data.servicePrefix + "-" + data.ClusterID + "-" + data.BaseDomain + "-hive-cluster",
		Description:            data.ClusterID + " - A managed hive created cluster",
		EscalationPolicy:       *escalationPolicy,
		AutoResolveTimeout:     &data.autoResolveTimeout,
		AcknowledgementTimeout: &data.acknowledgeTimeOut,
	}

	newSvc, err := client.CreateService(clusterService)
	if err != nil {
		return "", err
	}

	return newSvc.ID, nil
}

// DeleteService will get a service from the PD api and delete it
func (data *Data) DeleteService() error {
	id, err := data.GetService()
	if err != nil {
		// TODO Figure out how to handle if the PD Service is already deleted
		return nil
	}

	client := pdApi.NewClient(data.APIKey)
	err = client.DeleteService(id)
	return err
}
