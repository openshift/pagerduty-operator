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

func getDataKey(data map[string]string, key string) (string, error) {
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

func convertStrToUint(value string) (uint, error) {
	var retVal uint

	parsedU64, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return retVal, err
	}
	retVal = uint(parsedU64)

	return retVal, nil
}

// CreateService creates a service in pagerduty for the specified clusterid and returns the service key
func CreateService(osc client.Client, apiKey string, clusterid string, namespace string, configName string) (string, error) {
	pdConfigMap := &corev1.ConfigMap{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: configName}, pdConfigMap)
	if err != nil {
		return "", err
	}

	escalationPolicyID, err := getDataKey(pdConfigMap.Data, "ESCALATION_POLICY")
	if err != nil {
		return "", err
	}

	autoResolveTimeoutStr, err := getDataKey(pdConfigMap.Data, "RESOLVE_TIMEOUT")
	if err != nil {
		return "", err
	}
	autoResolveTimeout, err := convertStrToUint(autoResolveTimeoutStr)
	if err != nil {
		return "", err
	}

	acknowledgeTimeStr, err := getDataKey(pdConfigMap.Data, "ACKNOWLEDGE_TIMEOUT")
	if err != nil {
		return "", err
	}
	acknowledgeTimeOut, err := convertStrToUint(acknowledgeTimeStr)
	if err != nil {
		return "", err
	}

	client := pdApi.NewClient(apiKey)

	escalationPolicy, err := client.GetEscalationPolicy(string(escalationPolicyID), nil)
	if err != nil {
		return "", errors.New("Escalation policy not found in PagerDuty")
	}

	clusterService := pdApi.Service{
		Name:                   clusterid + "-hive-cluster",
		Description:            clusterid + " - A managed hive created cluster",
		EscalationPolicy:       *escalationPolicy,
		AutoResolveTimeout:     &autoResolveTimeout,
		AcknowledgementTimeout: &acknowledgeTimeOut,
	}

	newSvc, err := client.CreateService(clusterService)
	if err != nil {
		return "", err
	}

	return newSvc.ID, nil
}
