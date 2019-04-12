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

package clusterdeployment

import (
	"context"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivecontrollerutils "github.com/openshift/hive/pkg/controller/utils"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDeployment) handleCreate(request reconcile.Request, instance *hivev1.ClusterDeployment) (reconcile.Result, error) {
	r.reqLogger.Info("Creating syncset")

	if hivecontrollerutils.HasFinalizer(instance, "pd.manage.openshift.io/pagerduty") {
		return reconcile.Result{}, nil
	}

	hivecontrollerutils.AddFinalizer(instance, "pd.manage.openshift.io/pagerduty")
	err := r.client.Update(context.TODO(), instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	pdData := &pd.Data{
		ClusterID: instance.Name,
	}
	pdData.ParsePDConfig(r.client)
	pdServiceID, err := pdData.CreateService()
	if err != nil {
		return reconcile.Result{}, err
	}

	newSS := pdData.GenerateSyncSet(request.Namespace, request.Name, pdServiceID)

	if err := r.client.Create(context.TODO(), newSS); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
