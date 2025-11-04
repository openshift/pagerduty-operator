/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PagerDutyIntegrationSpec defines the desired state of PagerDutyIntegration
type PagerDutyIntegrationSpec struct {
	// Time in seconds that an incident changes to the Triggered State after
	// being Acknowledged. Value must not be negative. Omitting or setting
	// this field to 0 will disable the feature.
	// +kubebuilder:validation:Minimum=0
	AcknowledgeTimeout uint `json:"acknowledgeTimeout,omitempty"`

	// ID of an existing Escalation Policy in PagerDuty.
	EscalationPolicy string `json:"escalationPolicy"`

	// Time in seconds that an incident is automatically resolved if left
	// open for that long. Value must not be negative. Omitting or setting
	// this field to 0 will disable the feature.
	// +kubebuilder:validation:Minimum=0
	ResolveTimeout uint `json:"resolveTimeout,omitempty"`

	// Prefix to set on the PagerDuty Service name.
	ServicePrefix string `json:"servicePrefix"`

	// Reference to the secret containing PAGERDUTY_API_KEY.
	PagerdutyApiKeySecretRef corev1.SecretReference `json:"pagerdutyApiKeySecretRef"`

	// A label selector used to find which clusterdeployment CRs receive a
	// PD integration based on this configuration.
	ClusterDeploymentSelector metav1.LabelSelector `json:"clusterDeploymentSelector"`

	// Name and namespace in the target cluster where the secret is synced.
	TargetSecretRef corev1.SecretReference `json:"targetSecretRef"`

	//  The status of the serviceOrchestration and the referenced configmap resource
	ServiceOrchestration ServiceOrchestration `json:"serviceOrchestration,omitempty"`

	// Configures alert grouping for PD services
	AlertGroupingParameters *AlertGroupingParametersSpec `json:"alertGroupingParameters,omitempty"`

	// ID of the CAD service in PagerDuty.
	// If set, the operator will fetch the integration key for this service
	// and distribute it to clusters as CAD_PAGERDUTY_KEY.
	CADServiceID string `json:"cadServiceID,omitempty"`

	// ID of the CAD integration within the CAD service.
	// Required if CADServiceID is set.
	CADIntegrationID string `json:"cadIntegrationID,omitempty"`
}

// ServiceOrchestration defines if the service orchestration is enabled
// and the referenced configmap resource for the rules
type ServiceOrchestration struct {
	Enabled                bool                    `json:"enabled"`
	RuleConfigConfigMapRef *corev1.ObjectReference `json:"ruleConfigConfigMapRef,omitempty"`
}

// AlertGroupingParametersSpec defines the options used for alert grouping
type AlertGroupingParametersSpec struct {
	Type   string                             `json:"type,omitempty"`
	Config *AlertGroupingParametersConfigSpec `json:"config,omitempty"`
}

// AlertGroupingParametersConfigSpec defines the specifics for how an alert grouping type
// should behave
type AlertGroupingParametersConfigSpec struct {
	Timeout uint `json:"timeout,omitempty"`
}

// PagerDutyIntegrationStatus defines the observed state of PagerDutyIntegration
type PagerDutyIntegrationStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=pagerdutyintegrations,shortName=pdi,scope=Namespaced

// PagerDutyIntegration is the Schema for the pagerdutyintegrations API
type PagerDutyIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagerDutyIntegrationSpec   `json:"spec,omitempty"`
	Status PagerDutyIntegrationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PagerDutyIntegrationList contains a list of PagerDutyIntegration
type PagerDutyIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagerDutyIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagerDutyIntegration{}, &PagerDutyIntegrationList{})
}
