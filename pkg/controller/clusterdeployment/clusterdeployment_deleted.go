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
	"github.com/openshift/pagerduty-operator/config"
	metrics "github.com/openshift/pagerduty-operator/pkg/localmetrics"
	pd "github.com/openshift/pagerduty-operator/pkg/pagerduty"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDeployment) handleDelete(request reconcile.Request, instance *hivev1.ClusterDeployment) (reconcile.Result, error) {
	ClusterID := instance.Spec.ClusterName

	pdData := &pd.Data{
		ClusterID:  instance.Spec.ClusterName,
		BaseDomain: instance.Spec.BaseDomain,
	}
	err := pdData.ParsePDConfig(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = pdData.ParseClusterConfig(r.client, request.Namespace, request.Name)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.pdclient.DeleteService(pdData)
	if err != nil {
		r.reqLogger.Error(err, "Failed cleaning up pagerduty.")
	}

	if hivecontrollerutils.HasFinalizer(instance, config.OperatorFinalizer) {
		hivecontrollerutils.DeleteFinalizer(instance, config.OperatorFinalizer)
		err = r.client.Update(context.TODO(), instance)
		if err != nil {
			metrics.UpdateMetricPagerDutyDeleteFailure(1, ClusterID)
			return reconcile.Result{}, err
		}
	}
	metrics.UpdateMetricPagerDutyDeleteFailure(0, ClusterID)

	return reconcile.Result{}, nil
}
