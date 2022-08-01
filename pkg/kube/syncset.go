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

package kube

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
)

// GenerateSyncSet returns a syncset that can be created with the oc client
func GenerateSyncSet(namespace string, clusterDeploymentName string, secret *corev1.Secret, pdi *pagerdutyv1alpha1.PagerDutyIntegration) *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: namespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: clusterDeploymentName,
				},
			},
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: "Sync",
				Secrets: []hivev1.SecretMapping{
					{
						SourceRef: hivev1.SecretReference{
							Namespace: secret.Namespace,
							Name:      secret.Name,
						},
						TargetRef: hivev1.SecretReference{
							Namespace: pdi.Spec.TargetSecretRef.Namespace,
							Name:      pdi.Spec.TargetSecretRef.Name,
						},
					},
				},
			},
		},
	}
}

// GeneratePdSecret returns a secret that can be created with the oc client
func GeneratePdSecret(namespace string, name string, pdIntegrationKey string) *corev1.Secret {
	secret := &corev1.Secret{
		Type: "Opaque",
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			config.PagerDutySecretKey: []byte(pdIntegrationKey),
		},
	}

	return secret
}
