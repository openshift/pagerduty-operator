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

package pagerdutyintegration

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func Test_enqueueRequestForClusterDeployment_toRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	s := runtime.SchemeBuilder{
		corev1.AddToScheme,
		hivev1.AddToScheme,
		pagerdutyv1alpha1.AddToScheme,
	}
	assert.Nil(t, s.AddToScheme(scheme))

	tests := []struct {
		name             string
		obj              client.Object
		pdiObjs          []runtime.Object
		expectedRequests int
	}{
		{
			name:             "empty ClusterDeployment",
			obj:              &hivev1.ClusterDeployment{},
			pdiObjs:          []runtime.Object{},
			expectedRequests: 0,
		},
		{
			name: "empty ClusterDeployment",
			obj: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"pdiWatching": "clusterDeployment1",
					},
				},
			},
			pdiObjs: []runtime.Object{
				pagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "clusterDeployment1"}),
				pagerDutyIntegration("pdi2", map[string]string{"pdiWatching": "clusterDeployment2"}),
				pagerDutyIntegration("pdi3", map[string]string{"pdiWatching": "clusterDeployment1"}),
			},
			expectedRequests: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &enqueueRequestForClusterDeployment{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(test.obj).WithRuntimeObjects(test.pdiObjs...).Build(),
			}
			reqs := e.toRequests(test.obj)
			assert.Equal(t, test.expectedRequests, len(reqs))
		})
	}
}

func pagerDutyIntegration(name string, labels map[string]string) *pagerdutyv1alpha1.PagerDutyIntegration {
	return &pagerdutyv1alpha1.PagerDutyIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
		},
		Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{
			EscalationPolicy: "ABC123",
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			ServicePrefix: "test",
			PagerdutyApiKeySecretRef: corev1.SecretReference{
				Name:      "test",
				Namespace: "test",
			},
			TargetSecretRef: corev1.SecretReference{
				Name:      "test",
				Namespace: "test",
			},
		},
	}
}
