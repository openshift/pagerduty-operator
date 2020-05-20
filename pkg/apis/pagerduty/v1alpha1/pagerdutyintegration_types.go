package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PagerDutyIntegrationSpec defines the desired state of PagerDutyIntegration
// +k8s:openapi-gen=true
type PagerDutyIntegrationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html
}

// PagerDutyIntegrationStatus defines the observed state of PagerDutyIntegration
// +k8s:openapi-gen=true
type PagerDutyIntegrationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book.kubebuilder.io/beyond_basics/generating_crd.html
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PagerDutyIntegration is the Schema for the pagerdutyintegrations API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
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
