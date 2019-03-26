package pagerduty

import (
	"fmt"

	"github.com/openshift/pagerduty-operator/pkg/vault"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GenerateSyncSet returns the sync set for creation with the k8s go client
func GenerateSyncSet(osc client.Client, namespace string, name string) (*hivev1.SyncSet, error) {
	ssName := fmt.Sprintf("%v-pd-sync", name)

	vaultSecret, err := vault.GetVaultSecret(osc, "sre-pagerduty-operator", "vaultconfig", "whearn", "pagerduty")
	if err != nil {
		return nil, err
	}

	pdService, err := CreateService(osc, vaultSecret, name, namespace, "pagerduty-config")
	if err != nil {
		return nil, err
	}

	newSS := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName,
			Namespace: namespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: name,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: "upsert",
				Resources: []runtime.RawExtension{
					{
						Object: &corev1.Secret{
							Type: "Opaque",
							TypeMeta: metav1.TypeMeta{
								Kind:       "Secret",
								APIVersion: "v1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "pd-secret",
								Namespace: "openshift-am-config",
							},
							Data: map[string][]byte{
								"API_KEY": []byte(pdService),
							},
						},
					},
				},
			},
		},
	}

	return newSS, nil
}
