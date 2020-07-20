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

const (
	OperatorConfigMapName  string = "pagerduty-config"
	OperatorName           string = "pagerduty-operator"
	OperatorNamespace      string = "pagerduty-operator"
	PagerDutyAPISecretName string = "pagerduty-api-key"
	PagerDutyAPISecretKey  string = "PAGERDUTY_API_KEY"
	OperatorFinalizer      string = "pd.managed.openshift.io/pagerduty"
	SecretSuffix           string = "-pd-secret"
	ConfigMapSuffix        string = "-pd-config"

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
	// ClusterDeploymentNoalertsLabel is the label the clusterdeployment will have if the cluster should not send alerts
	ClusterDeploymentNoalertsLabel string = "api.openshift.com/noalerts"
)

// Name is used to generate the name of secondary resources (SyncSets,
// Secrets, ConfigMaps) for a ClusterDeployment that are created by
// the PagerDutyIntegration controller.
func Name(servicePrefix, clusterDeploymentName, suffix string) string {
	return servicePrefix + "-" + clusterDeploymentName + suffix
}
