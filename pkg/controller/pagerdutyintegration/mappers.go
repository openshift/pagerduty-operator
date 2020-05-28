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
	"context"
	"strings"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	pagerdutyv1alpha1 "github.com/openshift/pagerduty-operator/pkg/apis/pagerduty/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type clusterDeploymentToPagerDutyIntegrationsMapper struct {
	Client client.Client
}

func (m clusterDeploymentToPagerDutyIntegrationsMapper) Map(mo handler.MapObject) []reconcile.Request {
	pdiList := &pagerdutyv1alpha1.PagerDutyIntegrationList{}
	err := m.Client.List(context.TODO(), &client.ListOptions{}, pdiList)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, pdi := range pdiList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&pdi.Spec.ClusterDeploymentSelector)
		if err != nil {
			return []reconcile.Request{}
		}
		if selector.Matches(labels.Set(mo.Meta.GetLabels())) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pdi.Name,
					Namespace: pdi.Namespace,
				}},
			)
		}
	}
	return requests
}

type ownedByClusterDeploymentToPagerDutyIntegrationsMapper struct {
	Client client.Client
}

func (m ownedByClusterDeploymentToPagerDutyIntegrationsMapper) Map(mo handler.MapObject) []reconcile.Request {
	relevantClusterDeployments := []*hivev1.ClusterDeployment{}
	for _, or := range mo.Meta.GetOwnerReferences() {
		if or.APIVersion == hivev1.SchemeGroupVersion.String() && strings.ToLower(or.Kind) == "clusterdeployment" {
			cd := &hivev1.ClusterDeployment{}
			err := m.Client.Get(context.TODO(), client.ObjectKey{Name: or.Name, Namespace: mo.Meta.GetNamespace()}, cd)
			if err == nil {
				relevantClusterDeployments = append(relevantClusterDeployments, cd)
			}
		}
	}
	if len(relevantClusterDeployments) == 0 {
		return []reconcile.Request{}
	}

	pdiList := &pagerdutyv1alpha1.PagerDutyIntegrationList{}
	err := m.Client.List(context.TODO(), &client.ListOptions{}, pdiList)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, pdi := range pdiList.Items {
		selector, err := metav1.LabelSelectorAsSelector(&pdi.Spec.ClusterDeploymentSelector)
		if err != nil {
			return []reconcile.Request{}
		}

		for _, cd := range relevantClusterDeployments {
			if selector.Matches(labels.Set(cd.ObjectMeta.GetLabels())) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      pdi.Name,
						Namespace: pdi.Namespace,
					}},
				)
			}
		}
	}
	return requests
}
