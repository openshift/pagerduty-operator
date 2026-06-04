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
	"context"

	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/api/v1alpha1"
	"github.com/openshift/pagerduty-operator/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
		pdiObjs          []client.Object
		expectedRequests int
	}{
		{
			name:             "empty ClusterDeployment",
			obj:              &hivev1.ClusterDeployment{},
			pdiObjs:          []client.Object{},
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
			pdiObjs: []client.Object{
				mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "clusterDeployment1"}),
				mockPagerDutyIntegration("pdi2", map[string]string{"pdiWatching": "clusterDeployment2"}),
				mockPagerDutyIntegration("pdi3", map[string]string{"pdiWatching": "clusterDeployment1"}),
			},
			expectedRequests: 2,
		},
		{
			name: "PDI with In empty values matches nothing",
			obj: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"key1": "val1",
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-in-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpIn, Values: []string{}},
				}),
			},
			expectedRequests: 0,
		},
		{
			name: "PDI with NotIn empty values drops expression and matches",
			obj: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"key1": "val1",
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-notin-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpNotIn, Values: []string{}},
				}),
			},
			expectedRequests: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &enqueueRequestForClusterDeployment{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(test.obj).WithObjects(test.pdiObjs...).Build(),
			}
			reqs := e.toRequests(test.obj)
			assert.Equal(t, test.expectedRequests, len(reqs))
		})
	}
}

func Test_enqueueRequestForClusterDeploymentOwner_getAssociatedPagerDutyIntegrations(t *testing.T) {
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
		cdObjs           []client.Object
		pdiObjs          []client.Object
		expectedRequests int
	}{
		{
			name:             "Secret with no OwnerReference",
			obj:              &corev1.Secret{},
			cdObjs:           []client.Object{},
			pdiObjs:          []client.Object{},
			expectedRequests: 0,
		},
		{
			name: "ClusterDeployment",
			obj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret1",
					Namespace: "ns1",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "hive.openshift.io/v1",
							Kind:       "ClusterDeployment",
							Name:       "clusterDeployment1",
						},
					},
				},
			},
			cdObjs: []client.Object{
				&hivev1.ClusterDeployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterDeployment",
						APIVersion: "hive.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "clusterDeployment1",
						Namespace: "ns1",
						Labels: map[string]string{
							"pdiWatching": "clusterDeployment1",
						},
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "clusterDeployment1"}),
				mockPagerDutyIntegration("pdi2", map[string]string{"pdiWatching": "clusterDeployment2"}),
				mockPagerDutyIntegration("pdi3", map[string]string{"pdiWatching": "clusterDeployment1"}),
			},
			expectedRequests: 2,
		},
		{
			name: "PDI with In empty values matches nothing via owner",
			obj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-empty",
					Namespace: "ns1",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "hive.openshift.io/v1",
							Kind:       "ClusterDeployment",
							Name:       "cd-empty",
						},
					},
				},
			},
			cdObjs: []client.Object{
				&hivev1.ClusterDeployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterDeployment",
						APIVersion: "hive.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cd-empty",
						Namespace: "ns1",
						Labels: map[string]string{
							"key1": "val1",
						},
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-in-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpIn, Values: []string{}},
				}),
			},
			expectedRequests: 0,
		},
		{
			name: "PDI with NotIn empty values drops expression and matches via owner",
			obj: &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-notin-empty",
					Namespace: "ns1",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "hive.openshift.io/v1",
							Kind:       "ClusterDeployment",
							Name:       "cd-notin-empty",
						},
					},
				},
			},
			cdObjs: []client.Object{
				&hivev1.ClusterDeployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ClusterDeployment",
						APIVersion: "hive.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cd-notin-empty",
						Namespace: "ns1",
						Labels: map[string]string{
							"key1": "val1",
						},
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-notin-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpNotIn, Values: []string{}},
				}),
			},
			expectedRequests: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &enqueueRequestForClusterDeploymentOwner{
				Client: fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(test.obj).
					WithObjects(test.pdiObjs...).
					WithObjects(test.cdObjs...).
					Build(),
			}
			reqs := e.getAssociatedPagerDutyIntegrations(test.obj)
			assert.Equal(t, test.expectedRequests, len(reqs))
		})
	}
}

func Test_enqueueRequestForConfigmap_toRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	s := runtime.SchemeBuilder{
		corev1.AddToScheme,
		pagerdutyv1alpha1.AddToScheme,
	}
	assert.Nil(t, s.AddToScheme(scheme))

	tests := []struct {
		name             string
		obj              client.Object
		pdiObjs          []client.Object
		expectedRequests int
	}{
		{
			name:             "empty configmap",
			obj:              &corev1.ConfigMap{},
			pdiObjs:          []client.Object{},
			expectedRequests: 0,
		},
		{
			name: "PDI with In empty values matches nothing via configmap",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: config.OperatorNamespace,
					Labels: map[string]string{
						"key1": "val1",
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-in-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpIn, Values: []string{}},
				}),
			},
			expectedRequests: 0,
		},
		{
			name: "PDI with NotIn empty values drops expression via configmap",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-notin",
					Namespace: config.OperatorNamespace,
					Labels: map[string]string{
						"key1": "val1",
					},
				},
			},
			pdiObjs: []client.Object{
				mockPagerDutyIntegrationWithExpressions("pdi-notin-empty", []metav1.LabelSelectorRequirement{
					{Key: "key1", Operator: metav1.LabelSelectorOpNotIn, Values: []string{}},
				}),
			},
			expectedRequests: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := &enqueueRequestForConfigMap{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(test.obj).WithObjects(test.pdiObjs...).Build(),
			}
			reqs := e.toRequests(test.obj)
			assert.Equal(t, test.expectedRequests, len(reqs))
		})
	}
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	s := runtime.SchemeBuilder{
		corev1.AddToScheme,
		hivev1.AddToScheme,
		pagerdutyv1alpha1.AddToScheme,
	}
	if err := s.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

func Test_enqueueRequestForClusterDeployment_QueueMethods(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.TODO()

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cd1",
			Namespace: "ns1",
			Labels:    map[string]string{"pdiWatching": "cd1"},
		},
	}

	pdi := mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "cd1"})

	tests := []struct {
		name          string
		fire          func(handler *enqueueRequestForClusterDeployment, q workqueue.TypedRateLimitingInterface[reconcile.Request])
		expectedCount int
		expectedName  string
	}{
		{
			name: "Create enqueues matching PDI",
			fire: func(h *enqueueRequestForClusterDeployment, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Create(ctx, event.CreateEvent{Object: cd}, q)
			},
			expectedCount: 1,
			expectedName:  "pdi1",
		},
		{
			name: "Delete enqueues matching PDI",
			fire: func(h *enqueueRequestForClusterDeployment, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Delete(ctx, event.DeleteEvent{Object: cd}, q)
			},
			expectedCount: 1,
			expectedName:  "pdi1",
		},
		{
			name: "Generic enqueues matching PDI",
			fire: func(h *enqueueRequestForClusterDeployment, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Generic(ctx, event.GenericEvent{Object: cd}, q)
			},
			expectedCount: 1,
			expectedName:  "pdi1",
		},
		{
			name: "Create with no matching PDI enqueues nothing",
			fire: func(h *enqueueRequestForClusterDeployment, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				noMatchCD := &hivev1.ClusterDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cd-nomatch",
						Namespace: "ns1",
						Labels:    map[string]string{"other": "label"},
					},
				}
				h.Create(ctx, event.CreateEvent{Object: noMatchCD}, q)
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(cd).
				WithObjects(pdi).
				Build()

			handler := &enqueueRequestForClusterDeployment{Client: fakeClient}
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			tt.fire(handler, q)

			assert.Equal(t, tt.expectedCount, q.Len())
			if tt.expectedCount > 0 {
				req, _ := q.Get()
				assert.Equal(t, tt.expectedName, req.Name)
				q.Done(req)
			}
		})
	}
}

func Test_enqueueRequestForClusterDeployment_Update_Deduplication(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.TODO()

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cd1",
			Namespace: "ns1",
			Labels:    map[string]string{"pdiWatching": "cd1"},
		},
	}

	pdi := mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "cd1"})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cd).
		WithObjects(pdi).
		Build()

	handler := &enqueueRequestForClusterDeployment{Client: fakeClient}
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	handler.Update(ctx, event.UpdateEvent{ObjectOld: cd, ObjectNew: cd}, q)

	assert.Equal(t, 1, q.Len(), "same PDI from ObjectOld and ObjectNew should be de-duplicated")
}

func Test_enqueueRequestForClusterDeploymentOwner_QueueMethods(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.TODO()

	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDeployment",
			APIVersion: "hive.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cd1",
			Namespace: "ns1",
			Labels:    map[string]string{"pdiWatching": "cd1"},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "hive.openshift.io/v1",
					Kind:       "ClusterDeployment",
					Name:       "cd1",
				},
			},
		},
	}

	secretNoOwner := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret-no-owner",
			Namespace: "ns1",
		},
	}

	pdi := mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "cd1"})

	tests := []struct {
		name          string
		obj           client.Object
		expectedCount int
	}{
		{
			name:          "Create with owned secret enqueues matching PDI",
			obj:           secret,
			expectedCount: 1,
		},
		{
			name:          "Create with unowned secret enqueues nothing",
			obj:           secretNoOwner,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.obj, cd).
				WithObjects(pdi).
				Build()

			handler := &enqueueRequestForClusterDeploymentOwner{
				Client: fakeClient,
				Scheme: scheme,
			}
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			handler.Create(ctx, event.CreateEvent{Object: tt.obj}, q)

			assert.Equal(t, tt.expectedCount, q.Len())
		})
	}
}

func Test_enqueueRequestForConfigMap_QueueMethods(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.TODO()

	cmInOperatorNS := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orchestration-cm",
			Namespace: config.OperatorNamespace,
			Labels:    map[string]string{"pdiWatching": "cd1"},
		},
	}

	cmWrongNS := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orchestration-cm-wrong",
			Namespace: "other-namespace",
			Labels:    map[string]string{"pdiWatching": "cd1"},
		},
	}

	pdi := mockPagerDutyIntegration("pdi1", map[string]string{"pdiWatching": "cd1"})

	tests := []struct {
		name          string
		obj           client.Object
		fire          func(handler *enqueueRequestForConfigMap, q workqueue.TypedRateLimitingInterface[reconcile.Request])
		expectedCount int
		expectedName  string
	}{
		{
			name: "Create in operator namespace enqueues matching PDI",
			obj:  cmInOperatorNS,
			fire: func(h *enqueueRequestForConfigMap, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Create(ctx, event.CreateEvent{Object: cmInOperatorNS}, q)
			},
			expectedCount: 1,
			expectedName:  "pdi1",
		},
		{
			name: "Create in wrong namespace enqueues nothing",
			obj:  cmWrongNS,
			fire: func(h *enqueueRequestForConfigMap, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Create(ctx, event.CreateEvent{Object: cmWrongNS}, q)
			},
			expectedCount: 0,
		},
		{
			name: "Delete in operator namespace enqueues matching PDI",
			obj:  cmInOperatorNS,
			fire: func(h *enqueueRequestForConfigMap, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				h.Delete(ctx, event.DeleteEvent{Object: cmInOperatorNS}, q)
			},
			expectedCount: 1,
			expectedName:  "pdi1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.obj).
				WithObjects(pdi).
				Build()

			handler := &enqueueRequestForConfigMap{Client: fakeClient}
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
			defer q.ShutDown()

			tt.fire(handler, q)

			assert.Equal(t, tt.expectedCount, q.Len())
			if tt.expectedCount > 0 && tt.expectedName != "" {
				req, _ := q.Get()
				assert.Equal(t, types.NamespacedName{Name: tt.expectedName, Namespace: "test"}, req.NamespacedName)
				q.Done(req)
			}
		})
	}
}

func mockPagerDutyIntegrationWithExpressions(name string, exprs []metav1.LabelSelectorRequirement) *pagerdutyv1alpha1.PagerDutyIntegration {
	return &pagerdutyv1alpha1.PagerDutyIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test",
		},
		Spec: pagerdutyv1alpha1.PagerDutyIntegrationSpec{
			EscalationPolicy: "ABC123",
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchExpressions: exprs,
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

func mockPagerDutyIntegration(name string, labels map[string]string) *pagerdutyv1alpha1.PagerDutyIntegration {
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
