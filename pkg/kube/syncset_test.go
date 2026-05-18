package kube

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateSyncSet(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pd-secret",
			Namespace: "hive-ns",
		},
	}

	pdi := &pagerdutyv1alpha1.PagerDutyIntegration{
		Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{
			TargetSecretRef: corev1.SecretReference{
				Name:      "pd-secret",
				Namespace: "openshift-monitoring",
			},
		},
	}

	ss := GenerateSyncSet("hive-ns", "test-cluster", secret, pdi)

	assert.Equal(t, "test-pd-secret", ss.Name)
	assert.Equal(t, "hive-ns", ss.Namespace)
	assert.Equal(t, 1, len(ss.Spec.ClusterDeploymentRefs))
	assert.Equal(t, "test-cluster", ss.Spec.ClusterDeploymentRefs[0].Name)
	assert.Equal(t, hivev1.SyncSetResourceApplyMode("Sync"), ss.Spec.ResourceApplyMode)
	assert.Equal(t, 1, len(ss.Spec.Secrets))
	assert.Equal(t, "hive-ns", ss.Spec.Secrets[0].SourceRef.Namespace)
	assert.Equal(t, "test-pd-secret", ss.Spec.Secrets[0].SourceRef.Name)
	assert.Equal(t, "openshift-monitoring", ss.Spec.Secrets[0].TargetRef.Namespace)
	assert.Equal(t, "pd-secret", ss.Spec.Secrets[0].TargetRef.Name)
}

func TestGeneratePdSecret(t *testing.T) {
	secret := GeneratePdSecret("hive-ns", "test-pd-secret", "integration-key-123")

	assert.Equal(t, corev1.SecretType("Opaque"), secret.Type)
	assert.Equal(t, "Secret", secret.Kind)
	assert.Equal(t, "v1", secret.APIVersion)
	assert.Equal(t, "test-pd-secret", secret.Name)
	assert.Equal(t, "hive-ns", secret.Namespace)
	assert.Equal(t, []byte("integration-key-123"), secret.Data[config.PagerDutySecretKey])
}
