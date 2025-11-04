// Copyright 2018 RedHat
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

package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	OperatorConfigMapName  string = "pagerduty-config"
	OperatorName           string = "pagerduty-operator"
	OperatorNamespace      string = "pagerduty-operator"
	EnableOLMSkipRange     string = "true"
	PagerDutyAPISecretName string = "pagerduty-api-key" // #nosec G101 -- This is a false positive
	PagerDutyAPISecretKey  string = "PAGERDUTY_API_KEY" // #nosec G101 -- This is a false positive
	PagerDutySecretKey     string = "PAGERDUTY_KEY"     // #nosec G101 -- This is a false positive
	CADPagerDutySecretKey  string = "CAD_PAGERDUTY_KEY" // #nosec G101 -- This is a false positive
	// PagerDutyFinalizerPrefix prefix used for finalizers on resources other than PDI
	PagerDutyFinalizerPrefix string = "pd.managed.openshift.io/"
	// PagerDutyIntegrationFinalizer name of finalizer used for PDI
	PagerDutyIntegrationFinalizer string = "pd.managed.openshift.io/pagerduty"
	// LegacyPagerDutyFinalizer name of legacy finalizer, always to be deleted
	LegacyPagerDutyFinalizer string = "pd.managed.openshift.io/pagerduty"
	SecretSuffix             string = "-pd-secret"
	ConfigMapSuffix          string = "-pd-config"

	// PagerDutyUrgencyRule is the type of IncidentUrgencyRule for new incidents
	// coming into the Service. This is for the creation of NEW SERVICES ONLY
	// Supported values (by this operator) are:
	// * high - Treat all incidents as high urgency
	// * severity_based - Look to the severity on the PagerDuty Incident to map
	//   the urgency. An unset incident severity is equivalent to critical.
	PagerDutyUrgencyRule string = "severity_based"

	// ClusterDeploymentManagedLabel is the label the clusterdeployment will have that determines
	// if the cluster is OSD (managed) or not
	ClusterDeploymentManagedLabel string = "api.openshift.com/managed"

	// ClusterDeploymentLimitedSupportLabel is the label the clusterdeployment will have that determines
	// if the cluster's pagerduty service needs to enabled/disabled for alerting
	ClusterDeploymentLimitedSupportLabel string = "api.openshift.com/limited-support"

	// ClusterDeploymentSupportExceptionLabel is the label indicating the cluster is under a support
	// exception and the PagerDuty service should be enabled even if the cluster is in limited support
	ClusterDeploymentSupportExceptionLabel string = "ext-managed.openshift.io/support-exception"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the PagerDutyIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}

var isFedramp = false

// SetIsFedramp gets the value of fedramp
func SetIsFedramp() error {
	fedramp, ok := os.LookupEnv("FEDRAMP")
	if !ok {
		fedramp = "false"
	}

	fedrampBool, err := strconv.ParseBool(fedramp)
	if err != nil {
		return fmt.Errorf("invalid value for FedRAMP environment variable: %w", err)
	}

	isFedramp = fedrampBool
	return nil
}

// IsFedramp returns value of isFedramp var
func IsFedramp() bool {
	return isFedramp
}
