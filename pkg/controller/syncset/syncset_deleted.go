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

package syncset

import (
	"context"
	"strings"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileSyncSet) recreateSyncSet(request reconcile.Request) (reconcile.Result, error) {
	r.reqLogger.Info("Syncset deleted, regenerating")

	clusterdeployment := &hivev1.ClusterDeployment{}
	cdName := strings.Split(request.Name, "-")[0]

	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: request.Namespace, Name: cdName}, clusterdeployment)
	if err != nil {
		// Error finding the cluster deployment, requeue
		return reconcile.Result{}, err
	}

	pdData := &pd.Data{
		ClusterID:  clusterdeployment.Spec.ClusterName,
		BaseDomain: clusterdeployment.Spec.BaseDomain,
	}
	pdData.ParsePDConfig(r.client)

	// To prevent scoping issues in the err check below.
	var pdServiceID string

	pdServiceID, err = pdData.GetService()
	if err != nil {
		var createErr error
		pdServiceID, createErr = pdData.CreateService()
		if createErr != nil {
			return reconcile.Result{}, err
		}

	}

	newSS := pdData.GenerateSyncSet(request.Namespace, strings.Split(request.Name, "-")[0], pdServiceID)

	if err := r.client.Create(context.TODO(), newSS); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
