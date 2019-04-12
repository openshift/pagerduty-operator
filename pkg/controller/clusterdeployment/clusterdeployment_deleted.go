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
	"fmt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivecontrollerutils "github.com/openshift/hive/pkg/controller/utils"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDeployment) handleDelete(request reconcile.Request, instance *hivev1.ClusterDeployment) (reconcile.Result, error) {
	r.reqLogger.Info("Deleting syncset")

	ssName := fmt.Sprintf("%v-pd-sync", instance.Name)
	ss := &hivev1.SyncSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: ssName, Namespace: request.Namespace}, ss)
	if err != nil {
		if errors.IsNotFound(err) == false {
			return reconcile.Result{}, err
		}
	}

	pdData := &pd.Data{
		ClusterID: instance.Name,
	}
	err = pdData.ParsePDConfig(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = pdData.DeleteService()
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.client.Delete(context.TODO(), ss)
	if err != nil {
		return reconcile.Result{}, err
	}

	if hivecontrollerutils.HasFinalizer(instance, "pd.manage.openshift.io/pagerduty") {
		hivecontrollerutils.DeleteFinalizer(instance, "pd.manage.openshift.io/pagerduty")
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
