package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PagerDutyIntegrationSpec defines the desired state of PagerDutyIntegration
// +k8s:openapi-gen=true
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
}

// PagerDutyIntegrationStatus defines the observed state of PagerDutyIntegration
// +k8s:openapi-gen=true
type PagerDutyIntegrationStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PagerDutyIntegration is the Schema for the pagerdutyintegrations API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=pagerdutyintegrations,shortName=pdi,scope=Namespaced
type PagerDutyIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PagerDutyIntegrationSpec   `json:"spec,omitempty"`
	Status PagerDutyIntegrationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PagerDutyIntegrationList contains a list of PagerDutyIntegration
type PagerDutyIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PagerDutyIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PagerDutyIntegration{}, &PagerDutyIntegrationList{})
}
