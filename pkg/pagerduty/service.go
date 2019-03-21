package pagerduty

import (
	"context"
	"errors"
	"strconv"

	pdApi "github.com/PagerDuty/go-pagerduty"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateService creates a service in pagerduty for the specified clusterid and returns the service key
func CreateService(osc client.Client, apiKey string, clusterid string, namespace string, configName string) (string, error) {
	pdConfigMap := &corev1.ConfigMap{}
	err := osc.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: configName}, pdConfigMap)
	if err != nil {
		return "", err
	}

	escalationPolicyID, ok := pdConfigMap.Data["ESCALATION_POLICY"]
	if !ok {
		return "", errors.New("ESCALATION_POLICY is not set")
	}
	if len(escalationPolicyID) <= 0 {
		return "", errors.New("ESCALATION_POLICY is empty")
	}

	autoResolveTimeoutStr, ok := pdConfigMap.Data["RESOLVE_TIMEOUT"]
	if !ok {
		return "", errors.New("RESOLVE_TIMEOUT is not set")
	}
	if len(autoResolveTimeoutStr) <= 0 {
		return "", errors.New("RESOLVE_TIMEOUT is empty")
	}
	autoResolveTimeoutu64, err := strconv.ParseUint(string(autoResolveTimeoutStr), 10, 32)
	if err != nil {
		return "", errors.New("Error parsing RESOLVE_TIMEOUT")
	}
	autoResolveTimeout := uint(autoResolveTimeoutu64)

	acknowledgeTimeStr, ok := pdConfigMap.Data["ACKNOWLEDGE_TIMEOUT"]
	if !ok {
		return "", errors.New("ACKNOWLEDGE_TIMEOUT is not set")
	}
	if len(acknowledgeTimeStr) <= 0 {
		return "", errors.New("ACKNOWLEDGE_TIMEOUT is empty")
	}
	acknowledgeTimeOutu64, err := strconv.ParseUint(string(acknowledgeTimeStr), 10, 32)
	if err != nil {
		return "", errors.New("Error parsing ACKNOWLEDGE_TIMEOUT")
	}
	acknowledgeTimeOut := uint(acknowledgeTimeOutu64)

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
