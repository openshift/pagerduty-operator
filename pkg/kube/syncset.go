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
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GenerateSyncSet returns a syncset that can be created with the oc client
func GenerateSyncSet(namespace string, name string, pdIntegrationID string) *hivev1.SyncSet {
	ssName := name + config.SyncSetPostfix

	return &hivev1.SyncSet{
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
				ResourceApplyMode: "sync",
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
								Namespace: "openshift-monitoring",
							},
							Data: map[string][]byte{
								"PAGERDUTY_KEY": []byte(pdIntegrationID),
							},
						},
					},
				},
			},
		},
	}
}
