// Copyright 2020 Red Hat
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

package pagerdutyintegration

import (
	"testing"

	hiveapis "github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	pagerdutyapis "github.com/openshift/pagerduty-operator/pkg/apis"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestClusterDeploymentToPagerDutyIntegrationsMapper(t *testing.T) {
	assert.Nil(t, pagerdutyapis.AddToScheme(scheme.Scheme))
	assert.Nil(t, hiveapis.AddToScheme(scheme.Scheme))

	tests := []struct {
		name             string
		mapper           func(client client.Client) handler.Mapper
		objects          []runtime.Object
		mapObject        handler.MapObject
		expectedRequests []reconcile.Request
	}{
		{
			name:             "clusterDeploymentToPagerDutyIntegrations: empty",
			mapper:           clusterDeploymentToPagerDutyIntegrations,
			objects:          []runtime.Object{},
			mapObject:        handler.MapObject{},
			expectedRequests: []reconcile.Request{},
		},
		{
			name:   "clusterDeploymentToPagerDutyIntegrations: two matching PagerDutyIntegrations, one not matching",
			mapper: clusterDeploymentToPagerDutyIntegrations,
			objects: []runtime.Object{
				pagerDutyIntegration("test1", map[string]string{"test": "test"}),
				pagerDutyIntegration("test2", map[string]string{"test": "test"}),
				pagerDutyIntegration("test3", map[string]string{"notmatching": "test"}),
			},
			mapObject: handler.MapObject{
				Meta: &metav1.ObjectMeta{
					Labels: map[string]string{"test": "test"},
				},
			},
			expectedRequests: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      "test1",
						Namespace: "test",
					},
				},
				{
					NamespacedName: types.NamespacedName{
						Name:      "test2",
						Namespace: "test",
					},
				},
			},
		},

		{
			name:    "ownedByClusterDeploymentToPagerDutyIntegrations: empty",
			mapper:  ownedByClusterDeploymentToPagerDutyIntegrations,
			objects: []runtime.Object{},
			mapObject: handler.MapObject{
				Meta: &metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: hivev1.SchemeGroupVersion.String(),
						Kind:       "ClusterDeployment",
						Name:       "test",
						UID:        types.UID("test"),
					}},
				},
			},
			expectedRequests: []reconcile.Request{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewFakeClient(test.objects...)
			mapper := test.mapper(client)

			actualRequests := mapper.Map(test.mapObject)

			assert.Equal(t, test.expectedRequests, actualRequests)
		})
	}
}

func clusterDeploymentToPagerDutyIntegrations(client client.Client) handler.Mapper {
	return clusterDeploymentToPagerDutyIntegrationsMapper{Client: client}
}

func ownedByClusterDeploymentToPagerDutyIntegrations(client client.Client) handler.Mapper {
	return ownedByClusterDeploymentToPagerDutyIntegrationsMapper{Client: client}
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
			PagerdutyApiKeySecretRef: v1.SecretReference{
				Name:      "test",
				Namespace: "test",
			},
			TargetSecretRef: v1.SecretReference{
				Name:      "test",
				Namespace: "test",
			},
		},
	}
}
